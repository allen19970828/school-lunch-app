package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/xuri/excelize/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

//go:embed web/*
var webFS embed.FS

// JWT 密鑰
var jwtSecret = []byte("SchoolLunchV2_SuperSecretKey_2026")

// Claims JWT Payload 結構體
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"` // admin / teacher / accountant
	jwt.RegisteredClaims
}

// --- GORM 資料庫模型 ---

type Student struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ClassroomName string    `json:"classroom_name"`
	SeatNumber    int       `json:"seat_number"`
	StudentCode   string    `json:"student_code"`
	Name          string    `json:"name"`
	Identity      string    `json:"identity"`
	DietaryType   string    `json:"dietary_type"`
	Status        string    `json:"status"`
	IsMealUser    bool      `json:"is_meal_user"`
	CreatedAt     time.Time `json:"created_at"`
}

type Staff struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	MealMode     string    `json:"meal_mode"`
	IsSubsidized bool      `json:"is_subsidized"`
	SubReason    string    `json:"sub_reason"`
	CreatedAt    time.Time `json:"created_at"`
}

type MealCancellation struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TargetID     string    `json:"target_id"`
	TargetType   string    `json:"target_type"`
	TargetName   string    `json:"target_name"`
	TargetDetail string    `json:"target_detail"`
	CancelDate   string    `json:"cancel_date"`
	Reason       string    `json:"reason"`
	Status       string    `json:"status"`
	ReviewNote   string    `json:"review_note"`
	RefundAmount float64   `json:"refund_amount"`
	CreatedAt    time.Time `json:"created_at"`
}

type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Operator  string    `json:"operator"`
	Action    string    `json:"action"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

type MealComplaint struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Reporter  string    `json:"reporter"`
	Rating    int       `json:"rating"`
	DishName  string    `json:"dish_name"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// AuthMiddleware 正式 JWT 身份驗證與角色權限攔截器
func AuthMiddleware(requiredRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := c.Cookie("token")
		if err != nil || tokenString == "" {
			tokenString = c.GetHeader("Authorization")
			if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
				tokenString = tokenString[7:]
			}
		}

		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登入或未提供 Token (401 Unauthorized)"})
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "無效或已過期的 Token (401 Unauthorized)"})
			c.Abort()
			return
		}

		// 檢查角色權限
		if len(requiredRoles) > 0 {
			hasRole := false
			for _, role := range requiredRoles {
				if claims.Role == role {
					hasRole = true
					break
				}
			}
			if !hasRole {
				c.JSON(http.StatusForbidden, gin.H{"error": "您的權限不足無法執行此操作 (403 Forbidden)"})
				c.Abort()
				return
			}
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func main() {
	log.Println("🚀 正在啟動 school-lunch-app (含正式 JWT Auth & AuthMiddleware)...")

	db, err := gorm.Open(sqlite.Open("school_lunch_demo.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ 無法連線至 SQLite 資料庫: %v", err)
	}

	db.AutoMigrate(&Student{}, &Staff{}, &MealCancellation{}, &AuditLog{}, &MealComplaint{})

	// 預塞預設測試資料
	var studentCount int64
	db.Model(&Student{}).Count(&studentCount)
	if studentCount == 0 {
		db.Create(&Student{ClassroomName: "三年二班", SeatNumber: 1, StudentCode: "113001", Name: "林小明", Identity: "general", DietaryType: "omnivore", Status: "active", IsMealUser: true})
		db.Create(&Student{ClassroomName: "三年二班", SeatNumber: 2, StudentCode: "113002", Name: "陳美玲", Identity: "low_income", DietaryType: "vegan", Status: "active", IsMealUser: true})
	}

	var staffCount int64
	db.Model(&Staff{}).Count(&staffCount)
	if staffCount == 0 {
		db.Create(&Staff{Name: "張主任 (總務處)", Role: "行政主任", MealMode: "monthly", IsSubsidized: false})
		db.Create(&Staff{Name: "陳老師 (三年二班)", Role: "班導師", MealMode: "monthly", IsSubsidized: true, SubReason: "導師補助"})
		db.Create(&Staff{Name: "李組長 (午餐執秘)", Role: "午餐執秘", MealMode: "monthly", IsSubsidized: false})
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	subFS, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("無法載入靜態網頁: %v", err)
	}
	r.StaticFS("/ui", http.FS(subFS))

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/ui/")
	})

	api := r.Group("/api/v1")
	{
		// 🔑 1. 正式登入與 Token 簽發 API
		api.POST("/auth/login", func(c *gin.Context) {
			var loginReq struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := c.ShouldBindJSON(&loginReq); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "請輸入帳號與密碼"})
				return
			}

			// 驗證測試帳密
			var role string
			if loginReq.Username == "admin" && loginReq.Password == "admin123" {
				role = "admin"
			} else if loginReq.Username == "teacher" && loginReq.Password == "teacher123" {
				role = "teacher"
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "帳號或密碼錯誤！(提示: admin/admin123 或 teacher/teacher123)"})
				return
			}

			// 簽發 JWT Token
			expirationTime := time.Now().Add(24 * time.Hour)
			claims := &Claims{
				UserID:   1,
				Username: loginReq.Username,
				Role:     role,
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(expirationTime),
				},
			}

			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			tokenString, err := token.SignedString(jwtSecret)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "簽發 Token 失敗"})
				return
			}

			// 寫入 HTTP-Only Cookie
			c.SetCookie("token", tokenString, 86400, "/", "", false, true)

			db.Create(&AuditLog{
				Operator:  loginReq.Username,
				Action:    "使用者登入",
				Details:   fmt.Sprintf("帳號 %s 成功登入系統並取得 JWT Token", loginReq.Username),
				CreatedAt: time.Now(),
			})

			c.JSON(http.StatusOK, gin.H{
				"status":   "success",
				"token":    tokenString,
				"username": loginReq.Username,
				"role":     role,
				"message":  "登入成功！已成功簽發 24小時 JWT Token。",
			})
		})

		// 🔑 2. 登出 API
		api.POST("/auth/logout", func(c *gin.Context) {
			c.SetCookie("token", "", -1, "/", "", false, true)
			c.JSON(http.StatusOK, gin.H{"status": "success", "message": "已成功登出並清除 JWT Cookie。"})
		})

		// 🔑 3. 取得當前登入身分
		api.GET("/auth/me", AuthMiddleware(), func(c *gin.Context) {
			username, _ := c.Get("username")
			role, _ := c.Get("role")
			c.JSON(http.StatusOK, gin.H{
				"username": username,
				"role":     role,
			})
		})

		// --- 需驗證權限之受保護 API (Protected Routes) ---

		// 學生 CRUD (需登入)
		api.GET("/students", AuthMiddleware(), func(c *gin.Context) {
			var students []Student
			db.Order("classroom_name asc, seat_number asc").Find(&students)
			c.JSON(http.StatusOK, gin.H{"data": students})
		})

		// 新增/修改/刪除學生 (需超級管理員 admin 權限)
		api.POST("/students", AuthMiddleware("admin"), func(c *gin.Context) {
			var item Student
			if err := c.ShouldBindJSON(&item); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			item.Status = "active"
			item.IsMealUser = true
			item.CreatedAt = time.Now()
			db.Create(&item)
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
		})

		api.PUT("/students/:id", AuthMiddleware("admin"), func(c *gin.Context) {
			id := c.Param("id")
			var item Student
			if err := db.First(&item, id).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "找不到該學生"})
				return
			}
			_ = c.ShouldBindJSON(&item)
			db.Save(&item)
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
		})

		api.DELETE("/students/:id", AuthMiddleware("admin"), func(c *gin.Context) {
			id := c.Param("id")
			db.Delete(&Student{}, id)
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})

		// 教職員 CRUD (需登入)
		api.GET("/staff", AuthMiddleware(), func(c *gin.Context) {
			var staffList []Staff
			db.Order("id asc").Find(&staffList)
			c.JSON(http.StatusOK, gin.H{"data": staffList})
		})

		api.POST("/staff", AuthMiddleware("admin"), func(c *gin.Context) {
			var item Staff
			_ = c.ShouldBindJSON(&item)
			item.CreatedAt = time.Now()
			db.Create(&item)
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
		})

		api.PUT("/staff/:id", AuthMiddleware("admin"), func(c *gin.Context) {
			id := c.Param("id")
			var item Staff
			_ = db.First(&item, id)
			_ = c.ShouldBindJSON(&item)
			db.Save(&item)
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
		})

		api.DELETE("/staff/:id", AuthMiddleware("admin"), func(c *gin.Context) {
			id := c.Param("id")
			db.Delete(&Staff{}, id)
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})

		// 請假與審核 API
		api.GET("/cancellation/list", AuthMiddleware(), func(c *gin.Context) {
			var list []MealCancellation
			db.Order("id desc").Find(&list)
			c.JSON(http.StatusOK, gin.H{"data": list})
		})

		api.POST("/cancellation", AuthMiddleware(), func(c *gin.Context) {
			var req MealCancellation
			_ = c.ShouldBindJSON(&req)
			req.Status = "approved"
			req.RefundAmount = 60.0
			req.CreatedAt = time.Now()
			db.Create(&req)
			c.JSON(http.StatusOK, gin.H{"status": "approved", "data": req})
		})

		api.PUT("/cancellation/:id/status", AuthMiddleware("admin"), func(c *gin.Context) {
			id := c.Param("id")
			var req MealCancellation
			_ = db.First(&req, id)
			var input struct {
				Status     string `json:"status"`
				ReviewNote string `json:"review_note"`
			}
			_ = c.ShouldBindJSON(&input)
			req.Status = input.Status
			req.ReviewNote = input.ReviewNote
			db.Save(&req)
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": req})
		})

		// 稽核日誌 API
		api.GET("/audit-logs", AuthMiddleware("admin"), func(c *gin.Context) {
			var logs []AuditLog
			db.Order("id desc").Limit(20).Find(&logs)
			c.JSON(http.StatusOK, gin.H{"data": logs})
		})

		// 公文與結算 API
		api.POST("/procurement/draft", AuthMiddleware(), func(c *gin.Context) {
			draft := `【公務簽辦稿樣 - 免辦公開閱覽說明】
案由：臺中市神岡區豐洲國民小學辦理「115學年度學生午餐採購案」採購案，擬依《政府採購招標文件公開閱覽制度實施要點》第二點第3款規定免辦公開閱覽，簽請核示。`
			c.JSON(http.StatusOK, gin.H{"official_document_draft": draft})
		})

		api.POST("/settlement/export-excel", AuthMiddleware(), func(c *gin.Context) {
			f := excelize.NewFile()
			sheet := "收支結算表(114_04版)"
			f.SetSheetName("Sheet1", sheet)
			f.SetCellValue(sheet, "A1", "臺中市神岡區豐洲國民小學\n經費收支結算表")
			f.SetCellValue(sheet, "B6", 296010)
			f.SetCellValue(sheet, "E6", 284624)
			buffer, _ := f.WriteToBuffer()
			c.Header("Content-Disposition", "attachment; filename=\"豐洲國小_經費收支結算表.xlsx\"")
			c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
			c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
		})
	}

	fmt.Println("\n========================================================")
	fmt.Println("🎉 學校午餐 JWT Auth 正式身分驗證管理系統已成功啟動！")
	fmt.Println("🌐 請開啟瀏覽器訪問: http://localhost:8080/")
	fmt.Println("========================================================\n")

	if err := r.Run("0.0.0.0:8080"); err != nil {
		log.Fatalf("❌ 伺服器啟動失敗: %v", err)
	}
}
