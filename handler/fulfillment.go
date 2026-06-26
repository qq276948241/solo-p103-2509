package handler

import (
	"database/sql"
	"groupbuy/model"
	"groupbuy/response"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type SummaryItem struct {
	ProductID int64   `json:"product_id"`
	ProdName  string  `json:"prod_name"`
	Quantity  int     `json:"quantity"`
	TotalAmt  float64 `json:"total_amt"`
}

type SummaryOrder struct {
	OrderID  int64   `json:"order_id"`
	Name     string  `json:"name"`
	Phone    string  `json:"phone"`
	Address  string  `json:"address"`
	Remark   string  `json:"remark"`
	TotalAmt float64 `json:"total_amt"`
	Status   string  `json:"status"`
	Items    []gin.H `json:"items"`
}

func GroupSummary(c *gin.Context) {
	groupID := parseInt(c.Param("id"))
	if !groupExists(groupID) {
		response.Fail(c, response.CodeNotFound)
		return
	}

	prodRows, err := model.DB.Query(`
		SELECT oi.product_id, oi.prod_name, SUM(oi.quantity) as qty, SUM(oi.subtotal) as total
		FROM order_items oi
		JOIN orders o ON oi.order_id = o.id
		WHERE o.group_id = ? AND o.status != 'cancelled'
		GROUP BY oi.product_id
		ORDER BY oi.product_id
	`, groupID)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	defer prodRows.Close()

	var prodSummary []gin.H
	grandTotal := 0.0
	for prodRows.Next() {
		var pid int64
		var pName string
		var qty int
		var total float64
		if err := prodRows.Scan(&pid, &pName, &qty, &total); err != nil {
			continue
		}
		grandTotal += total
		prodSummary = append(prodSummary, gin.H{
			"product_id": pid, "prod_name": pName,
			"quantity": qty, "total_amt": total,
		})
	}
	if prodSummary == nil {
		prodSummary = []gin.H{}
	}

	orderRows, err := model.DB.Query(`
		SELECT id, phone, name, address, remark, total_amt, status
		FROM orders WHERE group_id = ? AND status != 'cancelled'
		ORDER BY id
	`, groupID)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	defer orderRows.Close()

	var orderSummary []gin.H
	for orderRows.Next() {
		var id int64
		var phone, name, addr, remark, status string
		var totalAmt float64
		if err := orderRows.Scan(&id, &phone, &name, &addr, &remark, &totalAmt, &status); err != nil {
			continue
		}
		items := loadOrderItems(id)
		orderSummary = append(orderSummary, gin.H{
			"order_id": id, "name": name, "phone": phone,
			"address": addr, "remark": remark, "total_amt": totalAmt,
			"status": status, "items": items,
		})
	}
	if orderSummary == nil {
		orderSummary = []gin.H{}
	}

	response.OK(c, gin.H{
		"group_id": groupID,
		"product_summary": prodSummary,
		"order_summary":   orderSummary,
		"grand_total":     grandTotal,
	})
}

type DeliveryItem struct {
	OrderID   int64   `json:"order_id"`
	Name      string  `json:"name"`
	Phone     string  `json:"phone"`
	Room      string  `json:"room"`
	Remark    string  `json:"remark"`
	ProdName  string  `json:"prod_name"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
	Subtotal  float64 `json:"subtotal"`
	Delivered bool    `json:"delivered"`
}

func DeliveryList(c *gin.Context) {
	groupID := parseInt(c.Param("id"))
	if !groupExists(groupID) {
		response.Fail(c, response.CodeNotFound)
		return
	}

	rows, err := model.DB.Query(`
		SELECT o.id, o.name, o.phone, o.address, o.remark,
		       oi.prod_name, oi.quantity, oi.unit_price, oi.subtotal,
		       CASE WHEN o.status = 'delivered' THEN 1 ELSE 0 END
		FROM orders o
		JOIN order_items oi ON o.id = oi.order_id
		WHERE o.group_id = ? AND o.status != 'cancelled'
		ORDER BY o.id
	`, groupID)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	defer rows.Close()

	buildingMap := map[string][]gin.H{}
	for rows.Next() {
		var orderID int64
		var name, phone, addr, remark, prodName string
		var qty int
		var unitPrice, subtotal float64
		var delivered int
		if err := rows.Scan(&orderID, &name, &phone, &addr, &remark, &prodName, &qty, &unitPrice, &subtotal, &delivered); err != nil {
			continue
		}
		building := extractBuilding(addr)
		item := gin.H{
			"order_id": orderID, "name": name, "phone": phone,
			"room": addr, "remark": remark, "prod_name": prodName,
			"quantity": qty, "unit_price": unitPrice, "subtotal": subtotal,
			"delivered": delivered == 1,
		}
		buildingMap[building] = append(buildingMap[building], item)
	}

	buildings := make([]string, 0, len(buildingMap))
	for b := range buildingMap {
		buildings = append(buildings, b)
	}
	sort.Slice(buildings, func(i, j int) bool {
		bi, ei := extractBuildingNum(buildings[i])
		bj, ej := extractBuildingNum(buildings[j])
		if ei == nil && ej == nil {
			return bi < bj
		}
		return buildings[i] < buildings[j]
	})

	result := make([]gin.H, 0, len(buildings))
	for _, b := range buildings {
		items := buildingMap[b]
		result = append(result, gin.H{
			"building": b,
			"items":    items,
		})
	}
	if result == nil {
		result = []gin.H{}
	}

	response.OK(c, result)
}

func MarkDelivered(c *gin.Context) {
	orderID := parseInt(c.Param("oid"))

	var status string
	err := model.DB.QueryRow("SELECT status FROM orders WHERE id = ?", orderID).Scan(&status)
	if err == sql.ErrNoRows {
		response.Fail(c, response.CodeNotFound)
		return
	}
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	if status == "cancelled" {
		response.Fail(c, response.CodeOrderCancelled)
		return
	}

	_, err = model.DB.Exec(
		"UPDATE orders SET status = 'delivered', updated_at = datetime('now','localtime') WHERE id = ?",
		orderID,
	)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}

	response.OK(c, gin.H{"order_id": orderID, "status": "delivered"})
}

func MarkUndelivered(c *gin.Context) {
	orderID := parseInt(c.Param("oid"))

	var status string
	err := model.DB.QueryRow("SELECT status FROM orders WHERE id = ?", orderID).Scan(&status)
	if err == sql.ErrNoRows {
		response.Fail(c, response.CodeNotFound)
		return
	}
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}

	_, err = model.DB.Exec(
		"UPDATE orders SET status = 'active', updated_at = datetime('now','localtime') WHERE id = ?",
		orderID,
	)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}

	response.OK(c, gin.H{"order_id": orderID, "status": "active"})
}

func extractBuilding(addr string) string {
	idx := strings.Index(addr, "号楼")
	if idx > 0 {
		return addr[:idx+6]
	}
	idx = strings.Index(addr, "栋")
	if idx > 0 {
		return addr[:idx+3]
	}
	parts := strings.Fields(addr)
	if len(parts) > 0 {
		return parts[0]
	}
	if len(addr) > 0 {
		return string(addr[0])
	}
	return "未知楼栋"
}

func extractBuildingNum(s string) (int, error) {
	i := 0
	for i < len(s) && !isDigit(s[i]) {
		i++
	}
	start := i
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	if start < i {
		return strconv.Atoi(s[start:i])
	}
	return 0, strconv.ErrSyntax
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
