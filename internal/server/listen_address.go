package server

import (
	"mega-magic/internal/config"
	"net"
	"strconv"
)

func httpListenAddress(cfg config.ServerConfig) string {
	return net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
}
