package main

import (
	"groupbuy/handler"
	"groupbuy/middleware"
	"groupbuy/model"
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/groupbuy.db"
	}

	if err := model.InitDB(dbPath); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer model.DB.Close()

	r := gin.Default()
	r.Use(corsMiddleware())

	api := r.Group("/api")

	api.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	admin := api.Group("/admin", middleware.LeaderAuth())
	{
		admin.POST("/groups", handler.CreateGroup)
		admin.GET("/groups", handler.ListGroups)
		admin.GET("/groups/:id", handler.GetGroup)
		admin.PUT("/groups/:id/close", handler.CloseGroup)

		admin.POST("/groups/:id/products", handler.AddProduct)
		admin.GET("/groups/:id/products", handler.ListProducts)
		admin.PUT("/groups/:id/products/:pid", handler.UpdateProduct)
		admin.PUT("/groups/:id/products/:pid/toggle", handler.ToggleProduct)

		admin.GET("/groups/:id/summary", handler.GroupSummary)
		admin.GET("/groups/:id/delivery", handler.DeliveryList)
		admin.PUT("/orders/:oid/deliver", handler.MarkDelivered)
		admin.PUT("/orders/:oid/undeliver", handler.MarkUndelivered)
	}

	neighbor := api.Group("", middleware.NeighborAuth())
	{
		neighbor.GET("/groups", handler.ListGroups)
		neighbor.GET("/groups/:id", handler.GetGroup)
		neighbor.GET("/groups/:id/products", handler.ListProducts)

		neighbor.POST("/groups/:id/orders", handler.CreateOrder)
		neighbor.GET("/groups/:id/orders", handler.ListMyOrders)
		neighbor.GET("/orders/:oid", handler.GetOrder)
		neighbor.PUT("/orders/:oid", handler.UpdateOrder)
		neighbor.DELETE("/orders/:oid", handler.CancelOrder)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("生鲜团购服务启动于 :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,X-Leader-Token,X-Phone")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
