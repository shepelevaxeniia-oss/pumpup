package main

import (
    "crypto/sha256"
    "crypto/hmac"
    "encoding/hex"
    "fmt"
    "math"
    "math/big"
    "math/rand"
    "time"
)

// GenerateServerSeed: создаёт случайный server seed (hex)
func GenerateServerSeed() string {
    b := make([]byte, 32)
    rand.Read(b)
    return hex.EncodeToString(b)
}

func ServerSeedHash(seed string) string {
    h := sha256.Sum256([]byte(seed))
    return hex.EncodeToString(h[:])
}

// DeriveFloat64 deterministically из serverSeed, clientSeed и roundId -> [0,1)
func DeriveFloat64(serverSeed, clientSeed, roundId string) float64 {
    // HMAC-SHA256(serverSeed, clientSeed|roundId)
    mac := hmac.New(sha256.New, []byte(serverSeed))
    mac.Write([]byte(clientSeed + "|" + roundId))
    sum := mac.Sum(nil)
    // interpret first 8 bytes as big int
    bigInt := new(big.Int).SetBytes(sum[:8])
    max := new(big.Int).Lsh(big.NewInt(1), 64) // 2^64
    f, _ := new(big.Rat).SetFrac(bigInt, max).Float64()
    return f
}

// Utility: compute multiplier for step (simple model). Can be replaced for RTP tuning.
func MultiplierAtStep(step int) float64 {
    // Example: mild progressive multiplier per step
    // multiplier = 1 + step * factor, factor tuned
    factor := 0.16667 // initial screenshot used x1.1667 at step 1-ish
    return 1.0 + float64(step)*factor
}

// Probability of survival to next step (simple decreasing model by difficulty scale)
func SurvivalProbabilityAtStep(step int, difficulty string) float64 {
    // difficulty: easy, medium, hard
    base := 0.96
    stepDecay := 0.02
    switch difficulty {
    case "easy":
        base = 0.96
        stepDecay = 0.015
    case "medium":
        base = 0.92
        stepDecay = 0.02
    case "hard":
        base = 0.88
        stepDecay = 0.025
    }
    p := base - float64(step)*stepDecay
    if p < 0.01 {
        p = 0.01
    }
    return p
}

// Decide if next step survives, given server/client/round
func IsNextStepSurvive(serverSeed, clientSeed, roundId string, step int, difficulty string) bool {
    v := DeriveFloat64(serverSeed, clientSeed, roundId + fmt.Sprintf("|%d", step))
    threshold := SurvivalProbabilityAtStep(step, difficulty)
    return v < threshold
}
