package main

import "time"

type User struct {
    ID string `json:"id"`
    Username string `json:"username"`
    CreatedAt time.Time `json:"created_at"`
}

type Round struct {
    ID string `json:"id"`
    UserID string `json:"user_id"`
    Stake int64 `json:"stake"`
    Step int `json:"step"`
    Multiplier float64 `json:"multiplier"`
    Status string `json:"status"`
    ServerSeed string `json:"server_seed"`
    ServerSeedHash string `json:"server_seed_hash"`
    ClientSeed string `json:"client_seed"`
}
