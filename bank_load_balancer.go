package main

import (
	pb "assignment_2/proto"
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"
)

// auth server
type BankServerInfo struct {
	Address string
	Load    float32
}

type BankLoadBalancer struct {
	pb.UnimplementedBankLoadBalancerServer
	//list of all available auth servers maintaining
	bankServers []BankServerInfo
	muty        sync.Mutex
}

// function to return all available bank servers
func (lb *BankLoadBalancer) GetAllBankServers(ctx context.Context, req *pb.Empty) (*pb.AllServersResponse, error) {
	lb.muty.Lock()
	defer lb.muty.Unlock()

	var servers []*pb.ServerInfo
	for _, server := range lb.bankServers {
		servers = append(servers, &pb.ServerInfo{Address: server.Address})
	}

	return &pb.AllServersResponse{Servers: servers}, nil
}

// function to register auth server
func (lb *BankLoadBalancer) RegisterBankServer(ctx context.Context, req *pb.ServerInfo) (*pb.Response, error) {
	lb.muty.Lock()
	defer lb.muty.Unlock()

	//adding this auth server with list of servers
	lb.bankServers = append(lb.bankServers, BankServerInfo{Address: req.Address, Load: 0.0})
	fmt.Println("Registered new bank server: ", req.Address)
	return &pb.Response{Status: "Bank server registered successfully"}, nil

}

// funtion to update cpu usage of bank server
func (lb *BankLoadBalancer) UpdateBankServerLoad(ctx context.Context, req *pb.BankServerLoad) (*pb.Response, error) {
	lb.muty.Lock()
	defer lb.muty.Unlock()

	for i := range lb.bankServers {
		//find that bank server
		if lb.bankServers[i].Address == req.Address {
			lb.bankServers[i].Load = float32(req.CpuUsage)
			fmt.Printf("Updated cpu usage for %s is: %.2f%%\n", req.Address, req.CpuUsage)
			return &pb.Response{Status: "Cpu load updated"}, nil
		}
	}

	return &pb.Response{Status: "Bank Sever not found sir"}, fmt.Errorf("Bank Server not found")
}

// function to get best auth server
func (lb *BankLoadBalancer) GetBankServer(ctx context.Context, req *pb.Empty) (*pb.ServerInfo, error) {
	lb.muty.Lock()
	defer lb.muty.Unlock()

	length := len(lb.bankServers)
	if length == 0 {
		return nil, fmt.Errorf("No Bank Server Available")
	}

	//least usage is best policy i follow
	mini := 0
	for ind, server := range lb.bankServers {
		if server.Load < lb.bankServers[mini].Load {
			mini = ind
		}
	}

	lb.bankServers[mini].Load++

	fmt.Println("Forwarding the request to bank server: ", lb.bankServers[mini].Address)
	return &pb.ServerInfo{Address: lb.bankServers[mini].Address}, nil
}

func main() {
	listen, err := net.Listen("tcp", ":50056")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	bankLB := &BankLoadBalancer{}

	//register with the bank load balancer
	pb.RegisterBankLoadBalancerServer(grpcServer, bankLB)

	fmt.Println("Bank Load balancer running on port 50056")
	errt := grpcServer.Serve(listen)
	if errt != nil {
		log.Fatalf("Failed to serve: %v", errt)
	}
}
