package transaction

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"eka-dev.cloud/transaction-service/config"
	inventorypb "eka-dev.cloud/transaction-service/pb/inventory"
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

// mockInventoryGrpcServer implements inventorypb.InventoryServiceServer for testing
type mockInventoryGrpcServer struct {
	inventorypb.UnimplementedInventoryServiceServer
}

func (m *mockInventoryGrpcServer) CheckStockAvailability(ctx context.Context, req *inventorypb.CheckStockRequest) (*inventorypb.CheckStockResponse, error) {
	return &inventorypb.CheckStockResponse{
		IsAvailable:  true,
		MissingItems: nil,
	}, nil
}

func (m *mockInventoryGrpcServer) DeductStock(ctx context.Context, req *inventorypb.DeductStockRequest) (*inventorypb.DeductStockResponse, error) {
	return &inventorypb.DeductStockResponse{
		Success: true,
		Message: "Stock deducted successfully",
	}, nil
}

func (m *mockInventoryGrpcServer) RollbackStock(ctx context.Context, req *inventorypb.RollbackStockRequest) (*inventorypb.RollbackStockResponse, error) {
	return &inventorypb.RollbackStockResponse{
		Success: true,
		Message: "Stock rolled back successfully",
	}, nil
}

func TestInventoryGrpcClient_SuccessWhenGrpcActive(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer lis.Close()

	grpcServer := grpc.NewServer()
	inventorypb.RegisterInventoryServiceServer(grpcServer, &mockInventoryGrpcServer{})
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()

	config.Config.EnableGrpc = true
	config.Config.ServiceInventoryGrpcUrl = lis.Addr().String()

	invClientLock.Lock()
	if invClientConn != nil {
		_ = invClientConn.Close()
	}
	invClientConn = nil
	invClient = nil
	invClientLock.Unlock()

	items := []*inventorypb.OrderItemPayload{
		{MenuId: 1, Quantity: 2},
	}
	res, err := checkStockResilient(context.Background(), items)
	assert.NoError(t, err)
	assert.True(t, res.IsAvailable)

	err = deductStockResilient(context.Background(), "tx-123", items)
	assert.NoError(t, err)
}

func TestInventoryGrpcClient_FallbackToHttpWhenGrpcOffline(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type checkResp struct {
			Success bool `json:"success"`
			Data    struct {
				IsAvailable  bool `json:"is_available"`
				MissingItems []struct {
					MenuID         int64   `json:"menu_id"`
					IngredientName string  `json:"ingredient_name"`
					StockAvailable float64 `json:"stock_available"`
					StockNeeded    float64 `json:"stock_needed"`
				} `json:"missing_items"`
			} `json:"data"`
		}
		resp := checkResp{
			Success: true,
		}
		resp.Data.IsAvailable = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer httpServer.Close()

	config.Config.EnableGrpc = true
	config.Config.ServiceInventoryGrpcUrl = "127.0.0.1:59999" // intentionally offline port
	config.Config.ServiceInventoryUrl = httpServer.URL
	config.Config.Secret = "dummy_secret"

	invClientLock.Lock()
	if invClientConn != nil {
		_ = invClientConn.Close()
	}
	invClientConn = nil
	invClient = nil
	invClientLock.Unlock()

	items := []*inventorypb.OrderItemPayload{
		{MenuId: 2, Quantity: 1},
	}
	res, err := checkStockResilient(context.Background(), items)
	assert.NoError(t, err)
	assert.True(t, res.IsAvailable)
}
