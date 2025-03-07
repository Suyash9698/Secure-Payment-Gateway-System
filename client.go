package main

import (
	pb "assignment_2/proto"
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var authLoadBalancerAddressForClient = "localhost:50055"
var paymentGatewayForClient = "localhost:50052"
var twoPhaseCommitServerForClient = "localhost:50057"
var transactionLoggerForClient = "localhost:50058"
var offlineQueueHandlerForClient = "localhost:50059"

// function that will create grpc connection
func connectGRPC(address string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to %s: %v", address, err)
	}
	return conn, err
}

// function that will register this user
func registerUser(authClient pb.AuthenticationClient, username, password, email string) string {
	resp, err := authClient.Register(context.Background(), &pb.ClientDetails{
		Username: username,
		Password: password,
		Email:    email,
	})

	if err != nil {
		log.Fatalf("Failed to register: %v", err)
	}

	fmt.Println("User registered successfully")
	return resp.Status
}

// function to login
func loginUser(authClient pb.AuthenticationClient, username, password string) string {
	resp, err := authClient.Login(context.Background(), &pb.Credentials{
		Username: username,
		Password: password,
	})

	if err != nil {
		log.Fatalf("Login failed :%v", err)
	}

	fmt.Println("Login successfully done!")
	return resp.Token
}

// **Function to create a gRPC context with the token**
func withAuthToken(token string) context.Context {
	md := metadata.New(map[string]string{"authorization": "Bearer " + token})
	return metadata.NewOutgoingContext(context.Background(), md)
}

// initiating a transaction sir
func initiateTransaction(pgClient pb.PaymentGatewayClient, token, sender, reciever string, amount float64) string {
	ctx := withAuthToken(token)
	resp, err := pgClient.InitiateTransaction(ctx, &pb.TransactionRequest{
		SenderId:   sender,
		RecieverId: reciever,
		Amount:     amount,
		Currency:   "USD",
	})
	if err != nil {
		log.Fatalf("Failed to initiate a transaction: %v", err)
	}
	fmt.Println("Transaction initiated sir: ", resp.TransactionId)
	return resp.TransactionId
}

// exexcuting 2 phase commit
func executeTwoPhaseCommit(tpClient pb.TwoPhaseCommitClient, token, transactionId, senderId, receiverId string, amount float64) {
	ctx := withAuthToken(token)
	//Phase 1: ready
	voteResp, err := tpClient.ReadyToCommitTransaction(ctx, &pb.TransactionDetails{
		TransactionId: transactionId,
		SenderId:      senderId,
		ReceiverId:    receiverId,
		Amount:        amount,
	})
	if err != nil {
		log.Fatalf("Ready to commit transaction failed: %v", err)
	}

	//Phase 2: abort or commit
	//means commit (all are ready)
	if voteResp.FinalDecision {
		commitResp, err := tpClient.CommitTransaction(context.Background(), &pb.TransactionDetails{
			TransactionId: transactionId,
			SenderId:      senderId,
			ReceiverId:    receiverId,
			Amount:        amount,
		})
		if err != nil {
			log.Fatalf("Failed to commit: %v", err)
		}
		fmt.Println("Successfully commited: ", commitResp)
	} else {
		abortResp, err := tpClient.AbortTransaction(context.Background(), &pb.TransactionID{TransactionId: transactionId})

		if err != nil {
			log.Fatalf("Failed to abort: %v", err)
		}
		fmt.Println("Successfully aborted: ", abortResp)
	}

}

// fetching transaction status
func getTransactionStatus(pgClient pb.PaymentGatewayClient, token, transactionId string) {
	ctx := withAuthToken(token)
	resp, err := pgClient.GetTransactionStatus(ctx, &pb.TransactionID{TransactionId: transactionId})
	if err != nil {
		log.Fatalf("Failed to get the transaction status: %v", err)
	}

	fmt.Println("Transaction status: ", resp.Status)
}

// processing offline transactions
func processOfflineTransactions(token string) {
	offlineConn, err := connectGRPC(offlineQueueHandlerForClient)
	if err != nil {
		log.Fatalf("Failed to connect to Offline Queue Handler: %v", err)
	}
	defer offlineConn.Close()

	// Create a client for the OfflineQueueService.
	offlineClient := pb.NewOfflineQueueServiceClient(offlineConn)

	ctx := withAuthToken(token)
	_, err = offlineClient.ProcessQueuedPayments(ctx, &pb.OfflineRequest{})
	if err != nil {
		log.Fatalf("Failed to process offline transactions: %v", err)
	}
	fmt.Println("Offline transactions processed successfully.")
}

// log transactions to that file
func logTransactions(loggerCLient pb.LoggingServiceClient, token, transactionId, clientId string, amount float64, status string) {
	ctx := withAuthToken(token)
	_, err := loggerCLient.LogTransaction(ctx, &pb.LogEntry{
		TransactionId: transactionId,
		ClientId:      clientId,
		Amount:        amount,
		Status:        status,
		Timestamp:     time.Now().Format(time.RFC3339),
	})

	if err != nil {
		fmt.Println("Failed to log transactions", err)
	}
	fmt.Println("Transaction Logged: ", transactionId, "| Status: ", status)
}

func main() {
	// First, connect to the Auth Load Balancer
	authLBConn, err := connectGRPC(authLoadBalancerAddressForClient)
	if err != nil {
		log.Fatalf("Failed to connect to Auth Load Balancer: %v", err)
	}
	defer authLBConn.Close()

	authLBClient := pb.NewAuthLoadBalancerClient(authLBConn)
	serverInfo, err := authLBClient.GetAuthServer(context.Background(), &pb.Empty{})
	if err != nil {
		log.Fatalf("Failed to get authentication server from load balancer: %v", err)
	}
	fmt.Println("Received Auth Server address:", serverInfo.Address)

	// Now connect to the actual Authentication server returned by the load balancer
	authConn, err := connectGRPC(serverInfo.Address)
	if err != nil {
		log.Fatalf("Failed to connect to Authentication Server: %v", err)
	}
	defer authConn.Close()
	authClient := pb.NewAuthenticationClient(authConn)

	// Connect to other services as before
	pgConn, err := connectGRPC(paymentGatewayForClient)
	if err != nil {
		log.Fatalf("Failed to connect to Payment Gateway: %v", err)
	}
	defer pgConn.Close()
	pgClient := pb.NewPaymentGatewayClient(pgConn)

	tpConn, err := connectGRPC(twoPhaseCommitServerForClient)
	if err != nil {
		log.Fatalf("Failed to connect to Two Phase Commit Server: %v", err)
	}
	defer tpConn.Close()
	tpClient := pb.NewTwoPhaseCommitClient(tpConn)

	loggerConn, err := connectGRPC(transactionLoggerForClient)
	if err != nil {
		log.Fatalf("Failed to connect to Transaction Logger: %v", err)
	}
	defer loggerConn.Close()
	loggerClient := pb.NewLoggingServiceClient(loggerConn)

	// Register user
	userName := "Suyash"
	passWord := "Suyash"
	email := "suyashkhareji@gmail.com"
	registerUser(authClient, userName, passWord, email)

	// Login user
	token := loginUser(authClient, userName, passWord)

	senderId := "SBI12345"
	receiverId := "SBI67890"
	amount := 100.0

	transactionId := initiateTransaction(pgClient, token, userName, "user", 100.0)

	executeTwoPhaseCommit(tpClient, token, transactionId, senderId, receiverId, amount)

	getTransactionStatus(pgClient, token, transactionId)

	logTransactions(loggerClient, token, transactionId, userName, 100.0, "Committed")

	processOfflineTransactions(token)

	fmt.Println("Client executed successfully!")
}
