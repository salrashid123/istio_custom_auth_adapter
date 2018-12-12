package mygrpcadapter

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"

	"istio.io/api/mixer/adapter/model/v1beta1"
	"istio.io/istio/mixer/template/authorization"
)

type (
	// Server is basic server interface
	Server interface {
		Addr() string
		Close() error
		Run(shutdown chan error)
	}

	// MyGrpcAdapter supports metric template.
	MyGrpcAdapter struct {
		listener net.Listener
		server   *grpc.Server
	}
)

var _ authorization.HandleAuthorizationServiceServer = &MyGrpcAdapter{}

func (s *MyGrpcAdapter) HandleAuthorization(ctx context.Context, r *authorization.HandleAuthorizationRequest) (*v1beta1.CheckResult, error) {
	return nil, nil
}

// Addr returns the listening address of the server
func (s *MyGrpcAdapter) Addr() string {
	return s.listener.Addr().String()
}

// Run starts the server run
func (s *MyGrpcAdapter) Run(shutdown chan error) {
	shutdown <- s.server.Serve(s.listener)
}

// Close gracefully shuts down the server; used for testing
func (s *MyGrpcAdapter) Close() error {
	if s.server != nil {
		s.server.GracefulStop()
	}

	if s.listener != nil {
		_ = s.listener.Close()
	}

	return nil
}

// NewMyGrpcAdapter creates a new IBP adapter that listens at provided port.
func NewMyGrpcAdapter(addr string) (Server, error) {
	if addr == "" {
		addr = "0"
	}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", addr))
	if err != nil {
		return nil, fmt.Errorf("unable to listen on socket: %v", err)
	}
	s := &MyGrpcAdapter{
		listener: listener,
	}
	fmt.Printf("listening on \"%v\"\n", s.Addr())
	s.server = grpc.NewServer()
	authorization.RegisterHandleAuthorizationServiceServer(s.server, s)
	return s, nil
}
