package runner

import (
	"context"
	"fmt"
	"net"

	sdk "github.com/PlakarKorp/integration-sdk"
)

type ConnCloseError struct {
	CloseErr     error
	ConnCloseErr error
}

func (e ConnCloseError) Error() string {
	closeErr := "ok"
	connErr := "ok"
	if e.CloseErr != nil {
		closeErr = e.CloseErr.Error()
	}
	if e.ConnCloseErr != nil {
		connErr = e.ConnCloseErr.Error()
	}
	return fmt.Sprintf("[close:%s] [conn:%s]", closeErr, connErr)
}

func closeConn(ctx context.Context, closeFn func(context.Context) error, conn net.Conn) error {
	var err ConnCloseError
	err.CloseErr = closeFn(ctx)
	err.ConnCloseErr = conn.Close()
	if err.CloseErr != nil || err.ConnCloseErr != nil {
		return err
	}
	return nil
}

type ImporterConn struct {
	sdk.Importer
	conn net.Conn
}

func (c *ImporterConn) Close(ctx context.Context) error {
	return closeConn(ctx, c.Importer.Close, c.conn)
}

type ExporterConn struct {
	sdk.Exporter
	conn net.Conn
}

func (c *ExporterConn) Close(ctx context.Context) error {
	return closeConn(ctx, c.Exporter.Close, c.conn)
}

type StoreConn struct {
	sdk.Store
	conn net.Conn
}

func (c *StoreConn) Close(ctx context.Context) error {
	return closeConn(ctx, c.Store.Close, c.conn)
}

type InventoryConn struct {
	sdk.Inventory
	conn net.Conn
}

func (c *InventoryConn) Close(ctx context.Context) error {
	return closeConn(ctx, c.Inventory.Close, c.conn)
}

type SecretProviderConn struct {
	sdk.SecretProvider
	conn net.Conn
}

func (c *SecretProviderConn) Close(ctx context.Context) error {
	return closeConn(ctx, c.SecretProvider.Close, c.conn)
}
