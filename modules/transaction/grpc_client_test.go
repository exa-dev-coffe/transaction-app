package transaction

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"eka-dev.cloud/transaction-service/config"
	masterdatapb "eka-dev.cloud/transaction-service/pb/masterdata"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// mockMasterDataGrpcServer implements masterdatapb.MasterDataServiceServer for testing
type mockMasterDataGrpcServer struct {
	masterdatapb.UnimplementedMasterDataServiceServer
}

func (m *mockMasterDataGrpcServer) GetAvailableMenusAndValidateTable(ctx context.Context, req *masterdatapb.GetMenusAvailableAndValidateTableRequest) (*masterdatapb.GetAvailableMenusResponse, error) {
	return &masterdatapb.GetAvailableMenusResponse{
		Menus: []*masterdatapb.MenuResponse{
			{
				Id:             99,
				Name:           "gRPC Espresso",
				Price:          25000,
				EffectivePrice: 20000,
				IsAvailable:    true,
			},
		},
	}, nil
}

func (m *mockMasterDataGrpcServer) GetMenusAndTable(ctx context.Context, req *masterdatapb.GetMenusAndTableRequest) (*masterdatapb.GetMenusAndTablesResponse, error) {
	return &masterdatapb.GetMenusAndTablesResponse{
		Menus: []*masterdatapb.MenuResponse{
			{
				Id:             99,
				Name:           "gRPC Espresso",
				Price:          25000,
				EffectivePrice: 20000,
			},
		},
		Tables: []*masterdatapb.TableResponse{
			{Id: 1, Name: "Table 1"},
		},
	}, nil
}

func TestGrpcClient_SuccessWhenGrpcActive(t *testing.T) {
	// 1. Start local mock gRPC server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer lis.Close()

	grpcServer := grpc.NewServer()
	masterdatapb.RegisterMasterDataServiceServer(grpcServer, &mockMasterDataGrpcServer{})
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()

	// 2. Point client to mock gRPC server
	config.Config.EnableGrpc = true
	config.Config.ServiceMasterDataGrpcUrl = lis.Addr().String()
	// Reset singleton connection for test
	mdClientLock.Lock()
	if mdClientConn != nil {
		_ = mdClientConn.Close()
	}
	mdClientConn = nil
	mdClient = nil
	mdClientLock.Unlock()

	// 3. Execute call
	menus, err := getAvailableMenusResilient(context.Background(), "99", 0)
	assert.NoError(t, err)
	assert.Len(t, menus, 1)
	assert.Equal(t, 99, menus[0].Id)
	assert.Equal(t, "gRPC Espresso", menus[0].Name)
	assert.Equal(t, 25000.0, menus[0].Price)
	assert.Equal(t, 20000.0, menus[0].EffectivePrice)
}

func TestGrpcClient_FallbackToHttpWhenGrpcOffline(t *testing.T) {
	// 1. Start local mock HTTP server simulating master-data-service REST API
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := InternalMenuResponse{
			Data: []MenuResponse{
				{
					Id:             88,
					Name:           "HTTP Fallback Latte",
					Price:          30000,
					EffectivePrice: 30000,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer httpServer.Close()

	// 2. Set gRPC to an unreachable offline port, but point HTTP URL to mock server
	config.Config.EnableGrpc = true
	config.Config.ServiceMasterDataGrpcUrl = "127.0.0.1:59999" // intentionally offline port
	config.Config.ServiceMasterDataUrl = httpServer.URL
	config.Config.Secret = "dummy_secret"

	// Reset singleton connection for test
	mdClientLock.Lock()
	if mdClientConn != nil {
		_ = mdClientConn.Close()
	}
	mdClientConn = nil
	mdClient = nil
	mdClientLock.Unlock()

	// 3. Execute call - should attempt gRPC, fail/timeout, and successfully fallback to HTTP REST!
	menus, err := getAvailableMenusResilient(context.Background(), "88", 0)
	assert.NoError(t, err)
	assert.Len(t, menus, 1)
	assert.Equal(t, 88, menus[0].Id)
	assert.Equal(t, "HTTP Fallback Latte", menus[0].Name)
}
