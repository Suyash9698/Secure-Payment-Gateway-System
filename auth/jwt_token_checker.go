package authee

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// secret key
var jwtSecret = []byte("suyash")

// function to check and cross verify the jwt token
func checkJWTToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		_, isOk := token.Method.(*jwt.SigningMethodHMAC)
		if !isOk {
			return nil, fmt.Errorf("Unexpected Signing Method")
		}
		return jwtSecret, nil
	})

	if err != nil {
		return "", err
	}

	//now we have to extract claims from token
	claimss, isOk := token.Claims.(jwt.MapClaims)

	if isOk && token.Valid {
		username, ok := claimss["username"].(string)
		if !ok {
			return "", fmt.Errorf("Invalid username field in token!")
		}
		return username, nil
	}

	return "", fmt.Errorf("Invalid Token sir!")
}

// enforce authentication
func AuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	mdata, isOk := metadata.FromIncomingContext(ctx)
	if !isOk {
		return nil, fmt.Errorf("Missing Metadata!")
	}

	authHeader, exists := mdata["authorization"]
	if !exists {
		return nil, fmt.Errorf("Missing Authorization token")
	}
	if len(authHeader) == 0 {
		return nil, fmt.Errorf("Missing Authorization token")
	}

	tokenStringHere := strings.TrimPrefix(authHeader[0], "Bearer ")

	username, err := checkJWTToken(tokenStringHere)

	if err != nil {
		return nil, fmt.Errorf("Invalid Token Sir G!")
	}

	fmt.Println("Authenticated User:", username)
	return handler(ctx, req)
}
