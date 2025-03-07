package main

import (
	pb "assignment_2/proto"
	"context"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
)

// bank server
type BankServer struct {
	pb.UnimplementedBankServerServer
	address string
}

// bank accounts : account number and balances
var bankAccounts = make(map[string]float64)
var mute sync.Mutex
var bankLoadBalancerAddress = "localhost:50056"

// function to abort the transaction for this server bank
func (b *BankServer) AbortTransaction(ctx context.Context, req *pb.TransactionID) (*pb.Response, error) {
	log.Printf("Transaction %s aborted: Rolling back changes", req.TransactionId)
	return &pb.Response{Status: "Aborted"}, nil
}

func getCpuUsage() (float64, error) {
	cmd := exec.Command("sh", "-c", "top -l 2 | grep 'CPU usage' | tail -n 1")
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("Error executing top command: %v", err)
	}

	line := strings.TrimSpace(string(out))
	parts := strings.Fields(line)

	length := len(parts)
	if length < 8 {
		return 0, fmt.Errorf("unexpected format: %s", line)
	}

	//format: "CPU usage: 12.34% user, 3.45% sys, 0.00% idle"
	//extracting idle percentage from above
	idleString := strings.TrimSuffix(parts[length-2], "%")
	idle, errrr := strconv.ParseFloat(idleString, 64)
	if errrr != nil {
		return 0, errrr
	}

	//to get cpu usage subtracting from 100%
	cpuU := 100.0 - idle
	return cpuU, nil

}

// seding this every 10th second
func (ser *BankServer) updateCpuUsage() {
	for {
		cpuUsage, err := getCpuUsage()
		if err != nil {
			fmt.Println("Error getting cpu usage")
			continue
		}
		//load balancer
		conn, err := grpc.Dial(bankLoadBalancerAddress, grpc.WithInsecure())
		if err != nil {
			fmt.Println("Failed to connect to auth load balancer")
			continue
		}

		client := pb.NewBankLoadBalancerClient(conn)
		client.UpdateBankServerLoad(context.Background(), &pb.BankServerLoad{
			Address:  ser.address,
			CpuUsage: cpuUsage,
		})

		if err != nil {
			fmt.Println("Failed to send cpu load: ", err)
		}

		conn.Close()
		time.Sleep(10 * time.Second)
	}
}

// function to register this server with bank load balancer
func registerWithBankLoadBalancer(address string) {
	conn, err := grpc.Dial(bankLoadBalancerAddress, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Failed to connect to bank load balancer: %v", err)
	}
	defer conn.Close()

	client := pb.NewBankLoadBalancerClient(conn)

	// now register this bank server with the load balancer
	_, err = client.RegisterBankServer(context.Background(), &pb.ServerInfo{Address: address})
	if err != nil {
		log.Fatalf("Failed to register bank server with load balancer: %v", err)
	}

	fmt.Printf("Bank server successfully registered with the load balancer at %s\n", bankLoadBalancerAddress)
}

// function to register the user with bank
func (ba *BankServer) RegisterUser(ctx context.Context, req *pb.ClientDetails) (*pb.Response, error) {
	mute.Lock()
	defer mute.Unlock()

	_, exists := bankAccounts[req.AccountNumber]
	if exists {
		return &pb.Response{Status: "User Already Registered in bank"}, nil
	}

	bankAccounts[req.AccountNumber] = 100 //100 rupees as a gift to open account with Suyash Bank of India

	fmt.Printf("Bank Account Created: Username: %s | Email: %s | Account: %s | Balance: ₹%.2f\n",
		req.Username, req.Email, req.AccountNumber, bankAccounts[req.AccountNumber])

	return &pb.Response{Status: "Bank account created successfully!"}, nil
}

// function to check whether user has enough amount or not
func (b *BankServer) HasEnoughMoney(ctx context.Context, req *pb.MoneyRequest) (*pb.MoneyResponse, error) {
	mute.Lock()
	defer mute.Unlock()

	balance, exists := bankAccounts[req.AccountNumber]
	if !exists {
		fmt.Println("Error:Account not found!")
		return &pb.MoneyResponse{Approved: false, Balance: 0}, fmt.Errorf("Account not found")
	}

	approved := false
	if balance > req.Amount {
		approved = true
	}

	return &pb.MoneyResponse{Approved: approved, Balance: balance}, nil
}

//function to deduct amount

func (b *BankServer) DeductMoney(ctx context.Context, req *pb.DeductRequest) (*pb.DeductResponse, error) {
	mute.Lock()
	defer mute.Unlock()

	moneyLeft, err := b.HasEnoughMoney(ctx, &pb.MoneyRequest{
		AccountNumber: req.AccountNumber,
		Amount:        req.Amount,
	})

	if err != nil {
		return &pb.DeductResponse{Success: false}, err
	}

	if moneyLeft.Approved {
		bankAccounts[req.AccountNumber] -= req.Amount
		fmt.Printf("Deducted %.2f amount. \n", req.Amount)
		return &pb.DeductResponse{Success: true}, nil
	}

	return &pb.DeductResponse{Success: false}, fmt.Errorf("Insufficient balance!")
}

// function to deposit money in their account
func (b *BankServer) DepositMoney(ctx context.Context, req *pb.DepositRequest) (*pb.DepositResponse, error) {
	mute.Lock()
	defer mute.Unlock()

	_, exists := bankAccounts[req.AccountNumber]

	if !exists {
		return &pb.DepositResponse{Success: false, NewBalance: -1}, fmt.Errorf("Account Not Found!")
	}

	bankAccounts[req.AccountNumber] += req.Amount

	fmt.Printf("Deposited Money %.2f into Account %s | New Balance: ₹%.2f\n",
		req.Amount, req.AccountNumber, bankAccounts[req.AccountNumber])

	return &pb.DepositResponse{Success: true, NewBalance: bankAccounts[req.AccountNumber]}, nil
}

func main() {
	listen, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("Failed to listen!")
	}

	port := listen.Addr().(*net.TCPAddr).Port
	addressH := fmt.Sprintf("localhost:%d", port)
	bankServer := BankServer{address: addressH}

	go registerWithBankLoadBalancer(addressH)

	//now report the cpu usage to bank load balancer
	go bankServer.updateCpuUsage()

	grpcServer := grpc.NewServer()
	pb.RegisterBankServerServer(grpcServer, &BankServer{})

	fmt.Println("Bank Server running on port ", port)
	errd := grpcServer.Serve(listen)
	if errd != nil {
		log.Fatalf("Failed to serve")
	}
}
