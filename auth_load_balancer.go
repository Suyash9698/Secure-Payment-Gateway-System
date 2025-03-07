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
type AuthServerInfo struct {
	Address string
	Load    float64
}

type AuthLoadBalancer struct {
	pb.UnimplementedAuthLoadBalancerServer
	//list of all available auth servers maintaining
	authServers map[string]*AuthServerInfo
	muty        sync.Mutex
}

// function to register auth server
func (lb *AuthLoadBalancer) RegisterAuthServer(ctx context.Context, req *pb.ServerInfo) (*pb.Response, error) {
	lb.muty.Lock()
	defer lb.muty.Unlock()

	//adding this auth server with list of servers
	lb.authServers[req.Address] = &AuthServerInfo{Address: req.Address, Load: 100}

	fmt.Println("Registered new auth server: ", req.Address)
	return &pb.Response{Status: "Auth server registered successfully"}, nil

}

// function to get best auth server
func (lb *AuthLoadBalancer) GetAuthServer(ctx context.Context, req *pb.Empty) (*pb.ServerInfo, error) {
	lb.muty.Lock()
	defer lb.muty.Unlock()

	length := len(lb.authServers)
	if length == 0 {
		return nil, fmt.Errorf("No Auth Server Available")
	}

	var ans *AuthServerInfo
	for _, server := range lb.authServers {
		if ans == nil || server.Load < ans.Load {
			ans = server
		}
	}

	fmt.Println("Forwarding the request to auth server: ", ans.Address)
	return &pb.ServerInfo{Address: ans.Address}, nil
}

// update krdo usage
func (lb *AuthLoadBalancer) UpdateAuthServerLoad(ctx context.Context, req *pb.AuthServerLoad) (*pb.Response, error) {
	lb.muty.Lock()
	defer lb.muty.Unlock()

	server, exists := lb.authServers[req.Address]
	if exists {
		server.Load = req.CpuUsage
		fmt.Printf("updated cpu load for %s: %.2f \n", req.Address, req.CpuUsage)
		return &pb.Response{Status: "CPU Usage updated successfully"}, nil
	}
	return &pb.Response{Status: "Auth server not found"}, fmt.Errorf("server not registered")
}

func main() {
	listen, err := net.Listen("tcp", ":50055")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	authLB := &AuthLoadBalancer{authServers: make(map[string]*AuthServerInfo)}
	pb.RegisterAuthLoadBalancerServer(grpcServer, authLB)

	fmt.Println("Auth Load balancer running on port 50055")
	errt := grpcServer.Serve(listen)
	if errt != nil {
		log.Fatalf("Failed to serve: %v", errt)
	}
}
