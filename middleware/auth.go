package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func AuthMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if this is a WebSocket upgrade request
		if c.GetHeader("Upgrade") == "websocket" {
			// For WebSocket, check token in query parameter or subprotocol
			token := c.Query("token")
			if token == "" {
				// Try to get from subprotocol
				subprotocols := c.GetHeader("Sec-WebSocket-Protocol")
				if subprotocols != "" {
					// Extract token from subprotocol if present
					// Format: "authorization.bearer.<token>"
					parts := strings.Split(subprotocols, ".")
					if len(parts) >= 3 && parts[0] == "authorization" && parts[1] == "bearer" {
						token = parts[2]
					}
				}
			}
			
			if token == "" {
				// For WebSocket without token, abort but don't write response
				// The WebSocket handler will handle the error
				c.Abort()
				return
			}
			
			// Validate token
			jwtToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(secret), nil
			})
			
			if err != nil || !jwtToken.Valid {
				// Invalid token, abort but don't write response
				// The WebSocket handler will handle the error
				c.Abort()
				return
			}
			
			// Token is valid, set user info
			if claims, ok := jwtToken.Claims.(jwt.MapClaims); ok {
				c.Set("user_id", uint(claims["user_id"].(float64)))
				c.Set("email", claims["email"].(string))
				c.Set("role", claims["role"].(string))
			}
			
			c.Next()
			return
		}
		
		// Regular HTTP request - check Authorization header or query parameter
		var tokenString string
		authHeader := c.GetHeader("Authorization")
		
		if authHeader != "" {
			// Extract token from "Bearer <token>"
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			}
		}
		
		// If no token in header, check query parameter (for MJPEG streaming with <img> tag)
		if tokenString == "" {
			tokenString = c.Query("token")
			// Also try GetQuery if Query doesn't work
			if tokenString == "" {
				if val, exists := c.GetQuery("token"); exists {
					tokenString = val
				}
			}
			// Debug: log if we're trying to read query param
			if tokenString != "" {
				fmt.Printf("[Auth] Token found in query parameter (length: %d)\n", len(tokenString))
			}
		}
		
		if tokenString == "" {
			// Debug: log what we received
			fmt.Printf("[Auth] No token found. Header: %s, Query: %s\n", authHeader, c.Query("token"))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
			c.Abort()
			return
		}
		
		// Parse and validate token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secret), nil
		})
		
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}
		
		// Extract claims
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			c.Set("user_id", uint(claims["user_id"].(float64)))
			c.Set("email", claims["email"].(string))
			c.Set("role", claims["role"].(string))
		}

		c.Next()
	}
}

