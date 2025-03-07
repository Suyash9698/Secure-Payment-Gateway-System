package main

import (
	pb "assignment_2/proto"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"google.golang.org/grpc"
)

type TransactionLogger struct {
	pb.UnimplementedLoggingServiceServer
	mu      sync.Mutex
	logFile *os.File
}

// function to initialize the log file
func NewTransactionLogger() *TransactionLogger {
	file, err := os.OpenFile("transactions.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Cannot open log file: %v", err)
	}

	return &TransactionLogger{logFile: file}
}

func (tl *TransactionLogger) LogTransaction(ctx context.Context, req *pb.LogEntry) (*pb.Response, error) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	logEntry := fmt.Sprintf("[%s] Transaction ID: %s | Client: %s | Amount: %.2f | Status: %s\n",
		req.Timestamp, req.TransactionId, req.ClientId, req.Amount, req.Status)

	_, err := tl.logFile.WriteString(logEntry)

	if err != nil {
		return &pb.Response{Status: "Log Failed"}, err
	}

	fmt.Println("Logged Transaction:", logEntry)
	return &pb.Response{Status: "Logged Successfully"}, nil

}

func main() {
	listen, err := net.Listen("tcp", ":50058")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	logger := NewTransactionLogger()
	pb.RegisterLoggingServiceServer(grpcServer, logger)

	fmt.Println("Transaction Logger running on port 50058")
	err = grpcServer.Serve(listen)
	if err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
