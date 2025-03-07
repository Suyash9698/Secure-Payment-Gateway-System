package main

import (
	pb "assignment_2/proto"
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
)

type AuthServer struct {
	pb.UnimplementedAuthenticationServer
	bankLoadBalancerAddress string
	address                 string
}

var users = make(map[string]*pb.ClientDetails)
var mu sync.Mutex

var jwtSecretKey = []byte("suyash")

var authLoadBalancerAddress = "localhost:50055"

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
func (ser *AuthServer) updateCpuUsage() {
	for {
		cpuUsage, err := getCpuUsage()
		if err != nil {
			fmt.Println("Error getting cpu usage")
			continue
		}
		//load balancer
		conn, err := grpc.Dial(authLoadBalancerAddress, grpc.WithInsecure())
		if err != nil {
			fmt.Println("Failed to connect to auth load balancer")
			continue
		}

		client := pb.NewAuthLoadBalancerClient(conn)
		client.UpdateAuthServerLoad(context.Background(), &pb.AuthServerLoad{
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

// first of all register with auth load balancer
func registerWithAuthLoadBalancer(address string) {
	conn, err := grpc.Dial(authLoadBalancerAddress, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect with auth load balancer")
	}

	defer conn.Close()

	client := pb.NewAuthLoadBalancerClient(conn)
	_, errer := client.RegisterAuthServer(context.Background(), &pb.ServerInfo{Address: address})
	if errer != nil {
		log.Fatalf("Failed to register with auth load balancer: %v", errer)
	}

	fmt.Println("Register with auth load balalncer successfully!")
}

// function to generate the 10 digit account number
func generateAccountNumber() string {
	return fmt.Sprintf("%010d", rand.Intn(1000000000))
}

// function to generate the jwt token
func generateJWTToken(username string) (string, error) {

	//token claim
	claim := jwt.MapClaims{
		"username":        username,
		"tokenExpireTime": time.Now().Add(time.Hour * 24).Unix(),
		"tokenIssueTime":  time.Now().Unix(),
	}

	//token create
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
	return token.SignedString(jwtSecretKey)
}

// function to get an available bank server
func getAvaialbleBankServer() (pb.BankServerClient, error) {
	//first of all connect with bank load balancer
	conn, err := grpc.Dial("localhost:50056", grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("Failed to connect with bank load balancer: %v", err)
	}

	defer conn.Close()

	client := pb.NewBankLoadBalancerClient(conn)
	bankServer, err := client.GetBankServer(context.Background(), &pb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("No available Bank Server")
	}

	//now try to connect with bank server
	bankConn, errf := grpc.Dial(bankServer.Address, grpc.WithInsecure())
	if errf != nil {
		return nil, fmt.Errorf("Failed to connect to selected Bank Server")
	}
	return pb.NewBankServerClient(bankConn), nil

}

// function to register the new user
func (ser *AuthServer) Register(ctx context.Context, req *pb.ClientDetails) (*pb.Response, error) {
	mu.Lock()
	defer mu.Unlock()

	_, exists := users[req.Username]

	if exists {
		return &pb.Response{Status: "Already Registered User"}, nil
	}

	//auotmatically generates account number
	accountNumber := generateAccountNumber()

	users[req.Username] = &pb.ClientDetails{
		Username:      req.Username,
		Password:      req.Password,
		Email:         req.Email,
		AccountNumber: accountNumber,
	}

	fmt.Println("User Registered with account number: ", accountNumber)

	//now call to get bank server possibly
	bankClient, err := getAvaialbleBankServer()
	if err != nil {
		return &pb.Response{Status: "User registered but no available bank server currently"}, nil
	}
	//register this user in bank server now:
	bankReq := &pb.ClientDetails{
		Username:      req.Username,
		AccountNumber: accountNumber,
	}

	_, ersr := bankClient.RegisterUser(ctx, bankReq)
	if ersr != nil {
		fmt.Println("Failed!: User registered but bank account creation failed!")
		return &pb.Response{Status: "User registered, but bank account failed!"}, nil
	}

	return &pb.Response{Status: "User registered successfully!"}, nil
}

// function to login handle the request of user
func (ser *AuthServer) Login(ctx context.Context, req *pb.Credentials) (*pb.AuthToken, error) {
	mu.Lock()
	defer mu.Unlock()

	user, exists := users[req.Username]
	if !exists {
		log.Printf("Error! Login attempt with invalid username: %s at %s", req.Username, time.Now().Format(time.RFC3339))
		return nil, fmt.Errorf("Invalid Username!")
	}

	//password check please
	if user.Password != req.Password {
		log.Printf("Error! Login attempt with invalid password for user: %s at %s", req.Username, time.Now().Format(time.RFC3339))
		return nil, fmt.Errorf("Invalid Password!")
	}

	//generate the token sir
	token, err := generateJWTToken(req.Username)

	if err != nil {
		return &pb.AuthToken{}, fmt.Errorf("Failed to generate token: %v", err)
	}

	log.Printf("User %s logged in successfully at %s", req.Username, time.Now().Format(time.RFC3339))

	return &pb.AuthToken{Token: token}, nil

}

func main() {
	listen, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	port := listen.Addr().(*net.TCPAddr).Port

	addressHere := fmt.Sprintf("localhost:%d", port)

	registerWithAuthLoadBalancer(addressHere)

	server := grpc.NewServer()
	authServer := &AuthServer{bankLoadBalancerAddress: "localhost:50055", address: addressHere}
	pb.RegisterAuthenticationServer(server, authServer)

	go authServer.updateCpuUsage()

	fmt.Println("gRPC Server is running on port ", port)
	errr := server.Serve(listen)
	if errr != nil {
		log.Fatalf("Failed to serve: %v", errr)
	}
}
