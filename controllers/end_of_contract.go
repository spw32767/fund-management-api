package controllers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"fund-management-api/config"
	"fund-management-api/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errEndOfContractContentRequired = errors.New("content is required")

type endOfContractCreateRequest struct {
	Content      string `json:"content" binding:"required"`
	DisplayOrder *int   `json:"display_order"`
}

type endOfContractUpdateRequest struct {
	Content      *string `json:"content"`
	DisplayOrder *int    `json:"display_order"`
}

type endOfContractReorderRequest struct {
	OrderedIDs []int `json:"ordered_ids"`
}

func fetchEndOfContractTerms() ([]models.EndOfContract, error) {
	var terms []models.EndOfContract
	if err := config.DB.
		Order("display_order ASC, eoc_id ASC").
		Find(&terms).Error; err != nil {
		return nil, err
	}
	return terms, nil
}

// GetEndOfContractTerms returns reward agreement terms for authenticated users.
func GetEndOfContractTerms(c *gin.Context) {
	terms, err := fetchEndOfContractTerms()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch end of contract terms"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    terms,
		"total":   len(terms),
	})
}

// GetEndOfContractTermsAdmin returns terms for admin management (same as public list).
func GetEndOfContractTermsAdmin(c *gin.Context) {
	GetEndOfContractTerms(c)
}

// CreateEndOfContractTerm creates a new agreement term.
func CreateEndOfContractTerm(c *gin.Context) {
	var req endOfContractCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Content is required"})
		return
	}

	var created models.EndOfContract
	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		newOrder := 0
		if req.DisplayOrder != nil && *req.DisplayOrder > 0 {
			newOrder = *req.DisplayOrder
		} else {
			var maxOrder sql.NullInt64
			if err := tx.Model(&models.EndOfContract{}).
				Select("COALESCE(MAX(display_order), 0)").
				Scan(&maxOrder).Error; err != nil {
				return err
			}
			newOrder = int(maxOrder.Int64) + 1
		}

		if newOrder < 1 {
			newOrder = 1
		}

		if err := tx.Model(&models.EndOfContract{}).
			Where("display_order >= ?", newOrder).
			Update("display_order", gorm.Expr("display_order + 1")).Error; err != nil {
			return err
		}

		created = models.EndOfContract{
			Content:      content,
			DisplayOrder: newOrder,
		}

		if err := tx.Create(&created).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create term"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Term created successfully",
		"data":    created,
	})
}

// UpdateEndOfContractTerm updates term content or display order.
func UpdateEndOfContractTerm(c *gin.Context) {
	idParam := c.Param("id")
	termID, err := strconv.Atoi(idParam)
	if err != nil || termID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid term ID"})
		return
	}

	var req endOfContractUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	if req.Content == nil && req.DisplayOrder == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
		return
	}

	err = config.DB.Transaction(func(tx *gorm.DB) error {
		var existing models.EndOfContract
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("eoc_id = ?", termID).
			First(&existing).Error; err != nil {
			return err
		}

		updates := make(map[string]interface{})

		if req.Content != nil {
			trimmed := strings.TrimSpace(*req.Content)
			if trimmed == "" {
				return errEndOfContractContentRequired
			}
			updates["content"] = trimmed
		}

		if req.DisplayOrder != nil {
			newOrder := *req.DisplayOrder
			if newOrder < 1 {
				newOrder = 1
			}
			if newOrder != existing.DisplayOrder {
				if newOrder < existing.DisplayOrder {
					if err := tx.Model(&models.EndOfContract{}).
						Where("display_order >= ? AND display_order < ? AND eoc_id <> ?", newOrder, existing.DisplayOrder, existing.EocID).
						Update("display_order", gorm.Expr("display_order + 1")).Error; err != nil {
						return err
					}
				} else {
					if err := tx.Model(&models.EndOfContract{}).
						Where("display_order <= ? AND display_order > ? AND eoc_id <> ?", newOrder, existing.DisplayOrder, existing.EocID).
						Update("display_order", gorm.Expr("display_order - 1")).Error; err != nil {
						return err
					}
				}
				updates["display_order"] = newOrder
				existing.DisplayOrder = newOrder
			}
		}

		if len(updates) == 0 {
			return nil
		}

		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Term not found"})
			return
		}
		if errors.Is(err, errEndOfContractContentRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": errEndOfContractContentRequired.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update term"})
		return
	}

	terms, fetchErr := fetchEndOfContractTerms()
	if fetchErr != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Term updated",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Term updated",
		"data":    terms,
	})
}

// DeleteEndOfContractTerm removes a term and resequences display order.
func DeleteEndOfContractTerm(c *gin.Context) {
	idParam := c.Param("id")
	termID, err := strconv.Atoi(idParam)
	if err != nil || termID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid term ID"})
		return
	}

	err = config.DB.Transaction(func(tx *gorm.DB) error {
		var existing models.EndOfContract
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("eoc_id = ?", termID).
			First(&existing).Error; err != nil {
			return err
		}

		if err := tx.Delete(&existing).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.EndOfContract{}).
			Where("display_order > ?", existing.DisplayOrder).
			Update("display_order", gorm.Expr("display_order - 1")).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Term not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete term"})
		return
	}

	terms, fetchErr := fetchEndOfContractTerms()
	if fetchErr != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Term deleted",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Term deleted",
		"data":    terms,
	})
}

// ReorderEndOfContractTerms updates ordering based on provided IDs.
func ReorderEndOfContractTerms(c *gin.Context) {
	var req endOfContractReorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	if len(req.OrderedIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ordered_ids is required"})
		return
	}

	seen := make(map[int]struct{}, len(req.OrderedIDs))
	for _, id := range req.OrderedIDs {
		if id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ordered_ids must contain positive integers"})
			return
		}
		if _, exists := seen[id]; exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ordered_ids must be unique"})
			return
		}
		seen[id] = struct{}{}
	}

	var total int64
	if err := config.DB.Model(&models.EndOfContract{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count terms"})
		return
	}

	if total > 0 && int64(len(req.OrderedIDs)) != total {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ordered_ids length does not match existing terms"})
		return
	}

	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		for index, id := range req.OrderedIDs {
			res := tx.Model(&models.EndOfContract{}).
				Where("eoc_id = ?", id).
				Update("display_order", index+1)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}
		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Term not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reorder terms"})
		return
	}

	terms, err := fetchEndOfContractTerms()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Display order updated",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Display order updated",
		"data":    terms,
	})
}
