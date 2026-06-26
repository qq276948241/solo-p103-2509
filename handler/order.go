package handler

import (
	"database/sql"
	"groupbuy/middleware"
	"groupbuy/model"
	"groupbuy/response"
	"fmt"

	"github.com/gin-gonic/gin"
)

type OrderItemInput struct {
	ProductID int64 `json:"product_id" binding:"required"`
	Quantity  int   `json:"quantity" binding:"required,min=1"`
}

type CreateOrderReq struct {
	Name    string          `json:"name" binding:"required"`
	Address string          `json:"address" binding:"required"`
	Remark  string          `json:"remark"`
	Items   []OrderItemInput `json:"items" binding:"required,min=1"`
}

func CreateOrder(c *gin.Context) {
	groupID := parseInt(c.Param("id"))
	if !groupExists(groupID) {
		response.Fail(c, response.CodeNotFound)
		return
	}

	if expired, msg := checkCutoff(groupID); expired {
		response.FailWithMsg(c, response.CodeCutoffPassed, msg)
		return
	}

	phone := middleware.GetPhone(c)
	var req CreateOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeParamError)
		return
	}

	tx, err := model.DB.Begin()
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	defer tx.Rollback()

	var totalAmt float64
	itemRows := make([]gin.H, 0, len(req.Items))
	prodCache := map[int64]gin.H{}

	for _, it := range req.Items {
		var pName string
		var pPrice float64
		var pStock, pOnShelf int
		err := tx.QueryRow(
			"SELECT name, unit_price, stock, on_shelf FROM products WHERE id = ? AND group_id = ?", it.ProductID, groupID,
		).Scan(&pName, &pPrice, &pStock, &pOnShelf)
		if err == sql.ErrNoRows {
			response.FailWithMsg(c, response.CodeNotFound, fmt.Sprintf("商品 %d 不存在", it.ProductID))
			return
		}
		if err != nil {
			response.Fail(c, response.CodeInternalError)
			return
		}
		if pOnShelf == 0 {
			response.FailWithMsg(c, response.CodeProductOffShelf, fmt.Sprintf("商品 %s 已下架", pName))
			return
		}

		if pStock > 0 {
			var sold int
			err = tx.QueryRow(`
				SELECT COALESCE(SUM(oi.quantity), 0)
				FROM order_items oi
				JOIN orders o ON oi.order_id = o.id
				WHERE oi.product_id = ? AND o.status != 'cancelled'
			`, it.ProductID).Scan(&sold)
			if err != nil {
				response.Fail(c, response.CodeInternalError)
				return
			}
			if sold+it.Quantity > pStock {
				response.FailWithMsg(c, response.CodeStockInsufficient,
					fmt.Sprintf("商品 %s 库存不足，已售 %d，上限 %d，还可购 %d", pName, sold, pStock, pStock-sold))
				return
			}
		}

		subtotal := pPrice * float64(it.Quantity)
		totalAmt += subtotal
		prodCache[it.ProductID] = gin.H{"name": pName, "price": pPrice}
		itemRows = append(itemRows, gin.H{
			"product_id": it.ProductID, "prod_name": pName,
			"quantity": it.Quantity, "unit_price": pPrice, "subtotal": subtotal,
		})
	}

	orderRes, err := tx.Exec(
		"INSERT INTO orders (group_id, phone, name, address, remark, total_amt, status) VALUES (?, ?, ?, ?, ?, ?, 'active')",
		groupID, phone, req.Name, req.Address, req.Remark, totalAmt,
	)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	orderID, _ := orderRes.LastInsertId()

	for _, it := range itemRows {
		_, err := tx.Exec(
			"INSERT INTO order_items (order_id, product_id, prod_name, quantity, unit_price, subtotal) VALUES (?, ?, ?, ?, ?, ?)",
			orderID, it["product_id"], it["prod_name"], it["quantity"], it["unit_price"], it["subtotal"],
		)
		if err != nil {
			response.Fail(c, response.CodeInternalError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}

	response.OK(c, gin.H{
		"order_id": orderID, "group_id": groupID, "phone": phone,
		"name": req.Name, "address": req.Address, "remark": req.Remark,
		"total_amt": totalAmt, "status": "active", "items": itemRows,
	})
}

func ListMyOrders(c *gin.Context) {
	groupID := parseInt(c.Param("id"))
	phone := middleware.GetPhone(c)

	rows, err := model.DB.Query(
		"SELECT id, group_id, phone, name, address, remark, total_amt, status, created_at, updated_at FROM orders WHERE group_id = ? AND phone = ? AND status != 'cancelled' ORDER BY id",
		groupID, phone,
	)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	defer rows.Close()

	var list []gin.H
	for rows.Next() {
		var id int64
		var gid int64
		var ph, name, addr, remark, status, createdAt, updatedAt string
		var totalAmt float64
		if err := rows.Scan(&id, &gid, &ph, &name, &addr, &remark, &totalAmt, &status, &createdAt, &updatedAt); err != nil {
			continue
		}
		items := loadOrderItems(id)
		list = append(list, gin.H{
			"id": id, "group_id": gid, "phone": ph, "name": name,
			"address": addr, "remark": remark, "total_amt": totalAmt,
			"status": status, "items": items, "created_at": createdAt, "updated_at": updatedAt,
		})
	}
	if list == nil {
		list = []gin.H{}
	}
	response.OK(c, list)
}

func GetOrder(c *gin.Context) {
	orderID := parseInt(c.Param("oid"))
	phone := middleware.GetPhone(c)

	var id int64
	var gid int64
	var ph, name, addr, remark, status, createdAt, updatedAt string
	var totalAmt float64
	err := model.DB.QueryRow(
		"SELECT id, group_id, phone, name, address, remark, total_amt, status, created_at, updated_at FROM orders WHERE id = ? AND phone = ?",
		orderID, phone,
	).Scan(&id, &gid, &ph, &name, &addr, &remark, &totalAmt, &status, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		response.Fail(c, response.CodeNotFound)
		return
	}
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}

	items := loadOrderItems(id)
	response.OK(c, gin.H{
		"id": id, "group_id": gid, "phone": ph, "name": name,
		"address": addr, "remark": remark, "total_amt": totalAmt,
		"status": status, "items": items, "created_at": createdAt, "updated_at": updatedAt,
	})
}

type UpdateOrderReq struct {
	Name    string          `json:"name"`
	Address string          `json:"address"`
	Remark  string          `json:"remark"`
	Items   []OrderItemInput `json:"items"`
}

func UpdateOrder(c *gin.Context) {
	orderID := parseInt(c.Param("oid"))
	phone := middleware.GetPhone(c)

	var gid int64
	var ordStatus string
	err := model.DB.QueryRow(
		"SELECT group_id, status FROM orders WHERE id = ? AND phone = ?", orderID, phone,
	).Scan(&gid, &ordStatus)
	if err == sql.ErrNoRows {
		response.Fail(c, response.CodeNotFound)
		return
	}
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}

	if ordStatus == "cancelled" {
		response.Fail(c, response.CodeOrderCancelled)
		return
	}

	if expired, msg := checkCutoff(gid); expired {
		response.FailWithMsg(c, response.CodeCutoffPassed, msg)
		return
	}

	var req UpdateOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeParamError)
		return
	}

	tx, err := model.DB.Begin()
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	defer tx.Rollback()

	updates := []string{}
	args := []interface{}{}
	if req.Name != "" {
		updates = append(updates, "name = ?")
		args = append(args, req.Name)
	}
	if req.Address != "" {
		updates = append(updates, "address = ?")
		args = append(args, req.Address)
	}
	if req.Remark != "" {
		updates = append(updates, "remark = ?")
		args = append(args, req.Remark)
	}

	var totalAmt float64

	if len(req.Items) > 0 {
		_, err := tx.Exec("DELETE FROM order_items WHERE order_id = ?", orderID)
		if err != nil {
			response.Fail(c, response.CodeInternalError)
			return
		}

		for _, it := range req.Items {
			var pName string
			var pPrice float64
			var pStock, pOnShelf int
			err := tx.QueryRow(
				"SELECT name, unit_price, stock, on_shelf FROM products WHERE id = ? AND group_id = ?", it.ProductID, gid,
			).Scan(&pName, &pPrice, &pStock, &pOnShelf)
			if err != nil {
				continue
			}
			if pOnShelf == 0 {
				continue
			}

			if pStock > 0 {
				var sold int
				err = tx.QueryRow(`
					SELECT COALESCE(SUM(oi.quantity), 0)
					FROM order_items oi
					JOIN orders o ON oi.order_id = o.id
					WHERE oi.product_id = ? AND o.status != 'cancelled' AND o.id != ?
				`, it.ProductID, orderID).Scan(&sold)
				if err != nil {
					response.Fail(c, response.CodeInternalError)
					return
				}
				if sold+it.Quantity > pStock {
					response.FailWithMsg(c, response.CodeStockInsufficient,
						fmt.Sprintf("商品 %s 库存不足，已售 %d，上限 %d，还可购 %d", pName, sold, pStock, pStock-sold))
					return
				}
			}

			subtotal := pPrice * float64(it.Quantity)
			totalAmt += subtotal
			_, err = tx.Exec(
				"INSERT INTO order_items (order_id, product_id, prod_name, quantity, unit_price, subtotal) VALUES (?, ?, ?, ?, ?, ?)",
				orderID, it.ProductID, pName, it.Quantity, pPrice, subtotal,
			)
			if err != nil {
				response.Fail(c, response.CodeInternalError)
				return
			}
		}
		updates = append(updates, "total_amt = ?")
		args = append(args, totalAmt)
	}

	if len(updates) > 0 {
		updates = append(updates, "updated_at = datetime('now','localtime')")
		args = append(args, orderID)
		query := "UPDATE orders SET " + joinSets(updates) + " WHERE id = ?"
		if _, err := tx.Exec(query, args...); err != nil {
			response.Fail(c, response.CodeInternalError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}

	response.OK(c, gin.H{"order_id": orderID})
}

func CancelOrder(c *gin.Context) {
	orderID := parseInt(c.Param("oid"))
	phone := middleware.GetPhone(c)

	var gid int64
	var ordStatus string
	err := model.DB.QueryRow(
		"SELECT group_id, status FROM orders WHERE id = ? AND phone = ?", orderID, phone,
	).Scan(&gid, &ordStatus)
	if err == sql.ErrNoRows {
		response.Fail(c, response.CodeNotFound)
		return
	}
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}

	if ordStatus == "cancelled" {
		response.Fail(c, response.CodeOrderCancelled)
		return
	}

	if expired, msg := checkCutoff(gid); expired {
		response.FailWithMsg(c, response.CodeCutoffPassed, msg)
		return
	}

	_, err = model.DB.Exec(
		"UPDATE orders SET status = 'cancelled', updated_at = datetime('now','localtime') WHERE id = ?",
		orderID,
	)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}

	response.OK(c, gin.H{"order_id": orderID, "status": "cancelled"})
}

func loadOrderItems(orderID int64) []gin.H {
	rows, err := model.DB.Query(
		"SELECT id, order_id, product_id, prod_name, quantity, unit_price, subtotal FROM order_items WHERE order_id = ?",
		orderID,
	)
	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()

	var items []gin.H
	for rows.Next() {
		var id, oid, pid int64
		var prodName string
		var qty int
		var unitPrice, subtotal float64
		if err := rows.Scan(&id, &oid, &pid, &prodName, &qty, &unitPrice, &subtotal); err != nil {
			continue
		}
		items = append(items, gin.H{
			"id": id, "order_id": oid, "product_id": pid,
			"prod_name": prodName, "quantity": qty,
			"unit_price": unitPrice, "subtotal": subtotal,
		})
	}
	if items == nil {
		items = []gin.H{}
	}
	return items
}
