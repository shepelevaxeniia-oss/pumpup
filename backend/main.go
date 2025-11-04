package main

import (
    "database/sql"
    "net/http"
    "os"
    "time"
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    _ "github.com/lib/pq"
    "log"
    "strconv"
)

var db *sql.DB

func main() {
    // load env
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        log.Fatal("set DATABASE_URL")
    }
    var err error
    db, err = sql.Open("postgres", dsn)
    if err != nil { log.Fatal(err) }
    db.SetMaxOpenConns(10)

    r := gin.Default()

    r.POST("/auth/login", loginHandler)
    r.GET("/balance", authMiddleware(), balanceHandler)
    r.POST("/rounds/start", authMiddleware(), startRoundHandler)
    r.POST("/rounds/step", authMiddleware(), stepHandler)
    r.POST("/rounds/cashout", authMiddleware(), cashoutHandler)
    r.GET("/logs", authMiddlewareAdmin(), logsHandler) // admin only

    port := os.Getenv("SERVER_PORT")
    if port == "" { port = "8080" }
    r.Run(":" + port)
}

// Dummy auth: in passport — JWT/ID. Here — header X-User-Id (uuid). In production use JWT.
func authMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        uid := c.GetHeader("X-User-Id")
        if uid == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error":"missing X-User-Id"})
            return
        }
        // verify user exists
        var exists bool
        row := db.QueryRow("select exists(select 1 from users where id=$1)", uid)
        row.Scan(&exists)
        if !exists {
            // create user for dev convenience
            _, err := db.Exec("insert into users (id, username) values ($1,$2)", uid, "dev-"+uid[:8])
            if err != nil {
                c.AbortWithStatusJSON(500, gin.H{"error":"cannot create user"})
                return
            }
            _, _ = db.Exec("insert into wallets (user_id, balance) values ($1,$2)", uid, 10000)
        }
        c.Set("user_id", uid)
        c.Next()
    }
}

func authMiddlewareAdmin() gin.HandlerFunc {
    return func(c *gin.Context) {
        // for simplicity use same header and a query param admin=true
        if c.Query("admin") != "true" {
            c.AbortWithStatusJSON(403, gin.H{"error":"admin only"})
            return
        }
        authMiddleware()(c)
    }
}

func loginHandler(c *gin.Context) {
    // Accept {"username":"..."} -> create user and return id
    var body struct{ Username string `json:"username"` }
    if err := c.BindJSON(&body); err != nil {
        c.JSON(400, gin.H{"error":"bad payload"})
        return
    }
    id := uuid.New().String()
    _, err := db.Exec("insert into users (id, username) values ($1,$2)", id, body.Username)
    if err != nil {
        c.JSON(500, gin.H{"error":"db error"})
        return
    }
    _, _ = db.Exec("insert into wallets (user_id, balance) values ($1,$2)", id, 10000)
    c.JSON(200, gin.H{"user_id": id})
}

func balanceHandler(c *gin.Context) {
    uid := c.GetString("user_id")
    var bal int64
    err := db.QueryRow("select balance from wallets where user_id=$1", uid).Scan(&bal)
    if err != nil {
        c.JSON(500, gin.H{"error":"db error"})
        return
    }
    c.JSON(200, gin.H{"balance": bal})
}

type StartReq struct {
    Stake int64 `json:"stake"`
    ClientSeed string `json:"client_seed"`
    Difficulty string `json:"difficulty"` // easy/medium/hard
}

func startRoundHandler(c *gin.Context) {
    uid := c.GetString("user_id")
    var r StartReq
    if err := c.BindJSON(&r); err != nil {
        c.JSON(400, gin.H{"error":"bad payload"})
        return
    }
    if r.Stake <= 0 {
        c.JSON(400, gin.H{"error":"invalid stake"})
        return
    }
    // check balance
    var bal int64
    err := db.QueryRow("select balance from wallets where user_id=$1", uid).Scan(&bal)
    if err != nil { c.JSON(500, gin.H{"error":"db"}) ; return }
    if bal < r.Stake {
        c.JSON(400, gin.H{"error":"insufficient funds"})
        return
    }
    // debit stake (idempotency not fully implemented)
    _, err = db.Exec("update wallets set balance = balance - $1 where user_id=$2", r.Stake, uid)
    if err != nil { c.JSON(500, gin.H{"error":"db update failed"}); return }

    roundId := uuid.New().String()
    serverSeed := GenerateServerSeed()
    hash := ServerSeedHash(serverSeed)
    // save round
    _, err = db.Exec(`insert into rounds (id,user_id,stake,step,multiplier,status,server_seed,server_seed_hash,client_seed,created_at,updated_at) 
        values ($1,$2,$3,0,1,'active',$4,$5,$6,now(),now())`,
        roundId, uid, r.Stake, serverSeed, hash, r.ClientSeed)
    if err != nil { c.JSON(500, gin.H{"error":"db insert failed"}); return }

    // log
    _, _ = db.Exec("insert into logs (round_id,user_id,event_type,payload) values ($1,$2,$3,$4)",
        roundId, uid, "round_started", `{"difficulty":"`+r.Difficulty+`"}`)

    c.JSON(200, gin.H{
        "round_id": roundId,
        "server_seed_hash": hash,
        "stake": r.Stake,
        "multiplier": 1.0,
        "step": 0,
    })
}

type StepReq struct {
    RoundID string `json:"round_id"`
    Difficulty string `json:"difficulty"`
}

func stepHandler(c *gin.Context) {
    uid := c.GetString("user_id")
    var r StepReq
    if err := c.BindJSON(&r); err != nil {
        c.JSON(400, gin.H{"error":"bad payload"}); return
    }
    // load round
    var round Round
    row := db.QueryRow("select id,user_id,stake,step,multiplier,status,server_seed,server_seed_hash,client_seed from rounds where id=$1", r.RoundID)
    if err := row.Scan(&round.ID, &round.UserID, &round.Stake, &round.Step, &round.Multiplier, &round.Status, &round.ServerSeed, &round.ServerSeedHash, &round.ClientSeed); err != nil {
        c.JSON(400, gin.H{"error":"round not found"}); return
    }
    if round.Status != "active" {
        c.JSON(400, gin.H{"error":"round not active"}); return
    }
    // decide survival
    survive := IsNextStepSurvive(round.ServerSeed, round.ClientSeed, round.ID, round.Step+1, r.Difficulty)
    if survive {
        // increment step and multiplier
        nextStep := round.Step + 1
        nextMul := MultiplierAtStep(nextStep)
        _, _ = db.Exec("update rounds set step=$1,multiplier=$2,updated_at=now() where id=$3", nextStep, nextMul, round.ID)
        _, _ = db.Exec("insert into logs (round_id,user_id,event_type,payload) values ($1,$2,$3,$4)", round.ID, uid, "step", `{"step":`+strconv.Itoa(nextStep)+`,"multiplier":`+strconv.FormatFloat(nextMul,'f',6,64)+`}`)
        c.JSON(200, gin.H{"result":"survived","step":nextStep,"multiplier":nextMul})
        return
    } else {
        // exploded -> round lost; status lost; stake already debited earlier
        _, _ = db.Exec("update rounds set status='lost',updated_at=now() where id=$1", round.ID)
        _, _ = db.Exec("insert into logs (round_id,user_id,event_type,payload) values ($1,$2,$3,$4)", round.ID, uid, "exploded", `{}`)
        c.JSON(200, gin.H{"result":"exploded","step": round.Step+1})
        return
    }
}

type CashoutReq struct {
    RoundID string `json:"round_id"`
}

func cashoutHandler(c *gin.Context) {
    uid := c.GetString("user_id")
    var r CashoutReq
    if err := c.BindJSON(&r); err != nil {
        c.JSON(400, gin.H{"error":"bad payload"}); return
    }
    var round Round
    row := db.QueryRow("select id,user_id,stake,step,multiplier,status,server_seed,server_seed_hash,client_seed from rounds where id=$1", r.RoundID)
    if err := row.Scan(&round.ID, &round.UserID, &round.Stake, &round.Step, &round.Multiplier, &round.Status, &round.ServerSeed, &round.ServerSeedHash, &round.ClientSeed); err != nil {
        c.JSON(400, gin.H{"error":"round not found"}); return
    }
    if round.Status != "active" {
        c.JSON(400, gin.H{"error":"round not active"}); return
    }
    // payout = stake * multiplier (multiplier from round.Step)
    payout := int64(math.Round(float64(round.Stake) * round.Multiplier))
    _, _ = db.Exec("update rounds set status='cashedout',updated_at=now() where id=$1", round.ID)
    _, _ = db.Exec("update wallets set balance = balance + $1 where user_id=$2", payout, uid)
    _, _ = db.Exec("insert into logs (round_id,user_id,event_type,payload) values ($1,$2,$3,$4)", round.ID, uid, "cashout", `{"payout":`+strconv.FormatInt(payout,10)+`}`)
    // publish revealed server seed so user can verify
    c.JSON(200, gin.H{"result":"cashedout","payout":payout,"server_seed": round.ServerSeed, "server_seed_hash": round.ServerSeedHash})
}

func logsHandler(c *gin.Context) {
    // return last 100 logs
    rows, err := db.Query("select id, round_id, user_id, event_type, payload, created_at from logs order by created_at desc limit 100")
    if err != nil { c.JSON(500, gin.H{"error":"db"}); return}
    defer rows.Close()
    var out []map[string]interface{}
    for rows.Next() {
        var id int
        var roundId, userId, etype string
        var payload []byte
        var createdAt time.Time
        rows.Scan(&id, &roundId, &userId, &etype, &payload, &createdAt)
        out = append(out, map[string]interface{}{
            "id": id, "round_id": roundId, "user_id": userId, "event_type": etype, "payload": string(payload), "created_at": createdAt,
        })
    }
    c.JSON(200, out)
}
