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

type OfflineQueueHandler struct {
	pb.UnimplementedOfflineQueueServiceServer
	queue []pb.TransactionRequest
	mu    sync.Mutex
}

var bankLoadBalancerAddress = "localhost:50056"

// ProcessQueuedPayments stores failed transactions in a queue for offline processing.
func (oqh *OfflineQueueHandler) ProcessQueuedPayments(ctx context.Context, req *pb.OfflineRequest) (*pb.Response, error) {
	oqh.mu.Lock()
	defer oqh.mu.Unlock()

	// Add the failed transactions to the queue.
	for _, trans := range req.Transactions {
		oqh.queue = append(oqh.queue, *trans)
	}

	fmt.Printf("Queued %d transactions for offline processing.\n", len(req.Transactions))
	return &pb.Response{Status: "Transactions queued for offline processing"}, nil
}

// retryFailedTransations periodically retries processing transactions in the queue.
func (oqh *OfflineQueueHandler) retryFailedTransations() {

	maxRetries := 5

	for {
		time.Sleep(10 * time.Second)

		oqh.mu.Lock()
		if len(oqh.queue) == 0 {
			oqh.mu.Unlock()
			continue
		}

		// process each queued transaction
		newQueue := []pb.TransactionRequest{}
		for _, trans := range oqh.queue {
			currRetries := 0
			currRetryDelay := 1 * time.Second
			success := false

			//exponential backoff retries
			for currRetries < maxRetries {
				bankServer, err := getAvailableBankServer()
				if err != nil {
					fmt.Println("No Available bank server sir currently! Retrying...")
				} else {
					//agar ho gya success
					if processTransaction(bankServer, trans) {
						success = true
						break
					}
				}
				currRetries++
				fmt.Printf("Attempt No. %d failed for %s, waiting %v...", currRetries, trans.TransactionId, currRetryDelay)
				time.Sleep(currRetryDelay)
				//exponential increasing delay here
				currRetryDelay *= 2
			}

			if !success {
				//keep in new queue it is not successful
				newQueue = append(newQueue, trans)
				fmt.Printf("Failed Transaction %s after %d retries", &trans.TransactionId, maxRetries)
			}
		}

		// updating the queue
		oqh.queue = newQueue
		oqh.mu.Unlock()
	}
}

// getAvailableBankServer returns a connected BankServerClient from the bank load balancer.
func getAvailableBankServer() (pb.BankServerClient, error) {
	conn, err := grpc.Dial(bankLoadBalancerAddress, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to bank load balancer: %v", err)
	}
	defer conn.Close()

	client := pb.NewBankLoadBalancerClient(conn)
	bankServer, err := client.GetBankServer(context.Background(), &pb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("Failed to connect with bank server")
	}

	// Connect to the selected bank server.
	bankConn, err := grpc.Dial(bankServer.Address, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("Failed to connect with bank server: %v", err)
	}

	return pb.NewBankServerClient(bankConn), nil
}

// processTransaction attempts to deduct money via the bank server.
func processTransaction(bankClient pb.BankServerClient, trans pb.TransactionRequest) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := bankClient.DeductMoney(ctx, &pb.DeductRequest{
		AccountNumber: trans.SenderId,
		Amount:        trans.Amount,
	})

	if err != nil {
		fmt.Printf("Transaction failed: %v\n", err)
		return false
	}
	if !resp.Success {
		fmt.Printf("Transaction failed: response unsuccessful\n")
		return false
	}

	fmt.Printf("Transaction succeeded! : %s\n", trans.TransactionId)
	return true
}

func main() {
	listen, err := net.Listen("tcp", ":50059")
	if err != nil {
		log.Fatalf("Failed to listen at this port: %v", err)
	}

	grpcServer := grpc.NewServer()
	offlineHandle := &OfflineQueueHandler{}

	// Register the new OfflineQueueService.
	pb.RegisterOfflineQueueServiceServer(grpcServer, offlineHandle)

	// Start background retry processing.
	go offlineHandle.retryFailedTransations()

	fmt.Println("Offline queue handler running on port 50059")
	if err := grpcServer.Serve(listen); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
