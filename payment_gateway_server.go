package main

import (
	authee "assignment_2/auth"
	pb "assignment_2/proto"
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/grpc"
)

type PaymentGatewayServer struct {
	pb.UnimplementedPaymentGatewayServer
}

// Now storing transactions in a map along with their statuses.
type TransactionInfo struct {
	Request *pb.TransactionRequest
	Status  string // e.g., "Pending", "Committed", "Aborted"
}

var transactions = make(map[string]*TransactionInfo)
var transMut sync.Mutex

// will keep track of the transactions processed so far
var processedTransations = make(map[string]bool)

// Initiates the transaction and stores it as "Pending"
func (pg *PaymentGatewayServer) InitiateTransaction(ctx context.Context, req *pb.TransactionRequest) (*pb.TransactionResponse, error) {
	transMut.Lock()
	defer transMut.Unlock()

	transactionId := req.TransactionId

	if transactionId == "" {
		transactionId = uuid.New().String()
	}

	_, alreadyExists := processedTransations[transactionId]

	if alreadyExists {
		return &pb.TransactionResponse{
			TransactionId: transactionId,
			Status:        "Duplicate Transaction! Already processed sir",
		}, nil
	}

	processedTransations[transactionId] = true

	transactions[transactionId] = &TransactionInfo{
		Request: req,
		Status:  "Pending",
	}
	fmt.Println("Transaction Initiated:", transactionId)
	return &pb.TransactionResponse{TransactionId: transactionId, Status: "Pending"}, nil
}

// New: GetTransactionStatus implementation to return the status of a transaction
func (pg *PaymentGatewayServer) GetTransactionStatus(ctx context.Context, req *pb.TransactionID) (*pb.TransactionStatus, error) {
	transMut.Lock()
	defer transMut.Unlock()

	tx, exists := transactions[req.TransactionId]
	if !exists {
		return nil, fmt.Errorf("Transaction not found")
	}
	// Return the stored status.
	return &pb.TransactionStatus{Status: tx.Status}, nil
}

func main() {
	listen, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Use interceptor for authentication if needed.
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(authee.AuthInterceptor))
	pb.RegisterPaymentGatewayServer(grpcServer, &PaymentGatewayServer{})

	fmt.Println("Payment Gateway Server running on port 50052...")
	err = grpcServer.Serve(listen)
	if err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
