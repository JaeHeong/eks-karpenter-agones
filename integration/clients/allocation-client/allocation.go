package allocation

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"open-match.dev/open-match/pkg/pb"
)

const GAME_MODE_SESSION = "mode.session"

type MatchRequest struct {
	Ticket     *pb.Ticket
	Tags       []string
	StringArgs map[string]string
}

type Player struct {
	UID          string
	MatchRequest *MatchRequest
}

func createRemoteClusterDialOption(clientCert, clientKey, caCert []byte) (grpc.DialOption, error) {
	cert, err := tls.X509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}}
	if len(caCert) != 0 {
		tlsConfig.RootCAs = x509.NewCertPool()
		tlsConfig.ServerName = "open-match-evaluator"
		if !tlsConfig.RootCAs.AppendCertsFromPEM(caCert) {
			return nil, errors.New("only PEM format is accepted for server CA")
		}
	}

	return grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), nil
}

var TicketID string // 전역 변수로 선언

func GetServerAssignment(omFrontendEndpoint string, room string, region string) string {
	log.Printf("Connecting to Open Match Frontend: " + omFrontendEndpoint)
	cert, err := os.ReadFile("public.cert")
	if err != nil {
		panic(err)
	}
	key, err := os.ReadFile("private.key")
	if err != nil {
		panic(err)
	}
	cacert, err := os.ReadFile("publicCA.cert")
	if err != nil {
		panic(err)
	}
	dialOpts, err := createRemoteClusterDialOption(cert, key, cacert)
	if err != nil {
		panic(err)
	}
	conn, err := grpc.Dial(omFrontendEndpoint, dialOpts)
	if err != nil {
		log.Fatalf("Failed to connect to Open Match Frontend, got %s", err.Error())
	}

	feService := pb.NewFrontendServiceClient(conn)

	player := &Player{
		UID: uuid.New().String(),
		MatchRequest: &MatchRequest{
			Tags: []string{GAME_MODE_SESSION},
			StringArgs: map[string]string{
				"room":   room,
				"region": region,
			},
		}}
	req := &pb.CreateTicketRequest{
		Ticket: &pb.Ticket{
			SearchFields: &pb.SearchFields{
				Tags:       player.MatchRequest.Tags,
				StringArgs: player.MatchRequest.StringArgs,
			},
		},
	}
	ticket, err := feService.CreateTicket(context.Background(), req)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	log.Printf("Ticket ID: %s\n", ticket.Id)
	TicketID = ticket.Id // 전역 변수에 티켓 ID 저장
	log.Printf("Waiting for ticket assignment")
	for {
		req := &pb.GetTicketRequest{
			TicketId: ticket.Id,
		}
		ticket, err := feService.GetTicket(context.Background(), req)

		if err != nil {
			return fmt.Sprintf("Was not able to get a ticket, err: %s\n", err.Error())
		}

		if ticket.Assignment != nil {
			log.Printf("Ticket assignment: %s\n", ticket.Assignment)
			log.Printf("Disconnecting from Open Match Frontend")

			defer conn.Close()
			return ticket.Assignment.Connection
		}
	}
}

func DeleteTicket(omFrontendEndpoint string, ticketID string) error {
	cert, err := os.ReadFile("public.cert")
	if err != nil {
		return err
	}
	key, err := os.ReadFile("private.key")
	if err != nil {
		return err
	}
	cacert, err := os.ReadFile("publicCA.cert")
	if err != nil {
		return err
	}
	dialOpts, err := createRemoteClusterDialOption(cert, key, cacert)
	if err != nil {
		return err
	}
	conn, err := grpc.Dial(omFrontendEndpoint, dialOpts)
	if err != nil {
		return fmt.Errorf("Failed to connect to Open Match Frontend, got %s", err)
	}
	defer conn.Close()

	feService := pb.NewFrontendServiceClient(conn)
	_, err = feService.DeleteTicket(context.Background(), &pb.DeleteTicketRequest{TicketId: ticketID})
	if err != nil {
		return fmt.Errorf("Failed to delete ticket: %v", err)
	}

	return nil
}
