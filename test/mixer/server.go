package mixer

import (
	"net"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"istio.io/pilot/test/mixer/pb"
)

func main() {

}

type server struct{}

func (s *server) Check(context.Context, *pb.CheckRequest) (*pb.CheckResponse, error) {
	return nil, nil
}

func (s *server) Report(context.Context, *pb.ReportRequest) (*pb.ReportResponse, error) {
	return nil, nil
}

func serve() error {
	lis, err := net.Listen("tcp", "80")
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	instance := &server{}
	pb.RegisterMixerServer(grpcServer, instance)
	grpcServer.Serve(lis)
	return nil
}
