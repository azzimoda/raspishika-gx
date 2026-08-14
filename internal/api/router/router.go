// Package router builds the HTTP router and registers all API routes.
package router

import (
	"github.com/azzimoda/raspishika-gx/internal/api/handler"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Init creates the gin engine with the API routes and the Swagger UI.
func Init(h *handler.Handler) *gin.Engine {
	r := gin.Default()

	api := r.Group("/api/v1")
	{
		api.GET("/departments", h.GetDepartments)
		api.GET("/groups", h.GetGroups)
		api.GET("/groups/:name", h.GetGroup)
		api.GET("/teachers", h.GetTeachers)
		api.GET("/teachers/search", h.SearchTeachers)
		api.GET("/teachers/:name_or_id", h.GetTeacher)
		api.GET("/schedule", h.GetSchedule)
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}
