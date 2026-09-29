package handler

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

type GeoMapHandler struct{}

func NewGeoMapHandler(_ interface{}) *GeoMapHandler {
    return &GeoMapHandler{}
}

func (h *GeoMapHandler) RegisterRoutes(r gin.IRouter, auth interface{}, tenant interface{}) {
    r.GET("/geo/topology-canvas", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"status": "corebit-topology-active"})
    })
}

func (h *GeoMapHandler) GetTopologyCanvas() interface{} {
    return gin.H{"status": "corebit-topology-active"}
}