package main

import (
	pb "assignment_2/proto"
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
)

type TwoPhaseCommitServer struct {
	pb.UnimplementedTwoPhaseCommitServer
	mu sync.Mutex
}

var bankLoadBalancerAddressHere = "localhost:50056"

func logTransaction(transactionId, clientId string, amount float64, status string) {
	conn, err := grpc.Dial("localhost:50058", grpc.WithInsecure())
	if err != nil {
		fmt.Println("Failed to connect to logging service")
		return
	}
	defer conn.Close()

	client := pb.NewLoggingServiceClient(conn)
	_, err = client.LogTransaction(context.Background(), &pb.LogEntry{
		TransactionId: transactionId,
		ClientId:      clientId,
		Amount:        amount,
		Status:        status,
		Timestamp:     time.Now().Format(time.RFC3339),
	})
	if err != nil {
		fmt.Println("Failed to log transaction:", err)
	}
}

// search for all available bank servers
func getAllBankServers() ([]pb.BankServerClient, []string, error) {
	conn, err := grpc.Dial(bankLoadBalancerAddressHere, grpc.WithInsecure())
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to connect to load balancer: %v", err)
	}

	defer conn.Close()

	client := pb.NewBankLoadBalancerClient(conn)
	bankServers, err := client.GetAllBankServers(context.Background(), &pb.Empty{})
	if err != nil {
		return nil, nil, fmt.Errorf("Currently no available bank servers")
	}
	//collecting all currently interacting clients
	var bankClients []pb.BankServerClient
	var bankAddresses []string

	for _, server := range bankServers.Servers {
		bankConn, err := grpc.Dial(server.Address, grpc.WithInsecure())
		if err != nil {
			log.Printf("Connection not established to this bank: %s", server.Address)
			continue
		}
		bankClients = append(bankClients, pb.NewBankServerClient(bankConn))
		bankAddresses = append(bankAddresses, server.Address)
	}

	return bankClients, bankAddresses, nil
}

// function to ask all bank servers if they are ready to commit
func (tpc *TwoPhaseCommitServer) ReadyToCommitTransaction(ctx context.Context, req *pb.TransactionDetails) (*pb.Vote, error) {
	tpc.mu.Lock()
	defer tpc.mu.Unlock()

	bankClients, bankAddresses, err := getAllBankServers()

	if err != nil {
		log.Printf("No available bank server ready for transaction: %s", req.TransactionId)
		return &pb.Vote{FinalDecision: false}, fmt.Errorf("No available bank servers")
	}
	if len(bankClients) == 0 {
		log.Printf("No available bank server ready for transaction: %s", req.TransactionId)
		return &pb.Vote{FinalDecision: false}, fmt.Errorf("No available bank servers")
	}

	//here setting timeout of 10 seconds
	timeOutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for i, bankClient := range bankClients {
		resp, err := bankClient.HasEnoughMoney(timeOutCtx, &pb.MoneyRequest{

			AccountNumber: req.SenderId,
			Amount:        req.Amount,
		})

		if err != nil || !resp.Approved {
			log.Printf("Bank Server %s not ready to commit %s", bankAddresses[i], req.TransactionId)
			return &pb.Vote{FinalDecision: false}, nil
		}
	}

	log.Printf("All Bank Servers are ready to commit: %s", req.TransactionId)
	return &pb.Vote{FinalDecision: true}, nil

}

// function to commit the transaction
func (tpc *TwoPhaseCommitServer) CommitTransaction(ctx context.Context, req *pb.TransactionDetails) (*pb.Response, error) {
	tpc.mu.Lock()
	defer tpc.mu.Unlock()

	bankClients, bankAddresses, err := getAllBankServers()

	if err != nil {
		log.Printf("No available bank server available for commiting transaction: %s", req.TransactionId)
		return &pb.Response{Status: "Commit Failed"}, fmt.Errorf("No available bank servers")
	}

	if len(bankClients) == 0 {
		log.Printf("No available bank server available for commiting transaction: %s", req.TransactionId)
		return &pb.Response{Status: "Commit Failed"}, fmt.Errorf("No available bank servers")
	}

	for i, bankClient := range bankClients {
		_, err := bankClient.DeductMoney(ctx, &pb.DeductRequest{
			AccountNumber: req.SenderId,
			Amount:        req.Amount,
		})

		if err != nil {
			log.Printf("Bank Server %s commit failed %s", bankAddresses[i], req.TransactionId)
			return &pb.Response{Status: "Commit Failed"}, err
		}
	}

	log.Printf("Commit Successful: %s", req.TransactionId)

	logTransaction(req.TransactionId, "SYSTEM", req.Amount, "Committed")

	return &pb.Response{Status: "Commit Successful!"}, err

}

// function to abort the transaction
func (tpc *TwoPhaseCommitServer) AbortTransaction(ctx context.Context, req *pb.TransactionID) (*pb.Response, error) {
	bankClients, bankAddresses, err := getAllBankServers()
	if err != nil || len(bankClients) == 0 {
		log.Printf("[AbortTransaction] No available bank servers for aborting transaction %s", req.TransactionId)
		return &pb.Response{Status: "Abort Failed"}, fmt.Errorf("No available bank servers")
	}

	for i, bankClient := range bankClients {
		_, err := bankClient.AbortTransaction(ctx, req)
		if err != nil {
			log.Printf("Failed to notify bank server %s about abort", bankAddresses[i])
		}
	}

	log.Printf("Transaction %s aborted", req.TransactionId)

	logTransaction(req.TransactionId, "SYSTEM", 0, "Aborted")

	return &pb.Response{Status: "Aborted"}, nil

}

func main() {
	listen, err := net.Listen("tcp", ":50057")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	tpServer := &TwoPhaseCommitServer{}

	pb.RegisterTwoPhaseCommitServer(grpcServer, tpServer)

	fmt.Println("Two-Phase Commit Server running on port 50057")
	err = grpcServer.Serve(listen)
	if err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

}
