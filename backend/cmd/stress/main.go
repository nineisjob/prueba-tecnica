// Command stress empirically proves the concurrency engine: it fires a
// configurable number of simultaneous bid requests at the same auction and
// verifies, against the live API (not local counters), that exactly the
// rules in domain.Auction.ValidateBid held under real concurrent load.
//
// It defines its own minimal request/response structs mirroring the wire
// format documented in the API contract, rather than importing
// transport/http's unexported DTOs: this tool is, deliberately, just
// another HTTP client -- the same contract a browser or curl would see.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type config struct {
	base            string
	users           int
	concurrency     int
	mode            string
	durationSeconds int
	incrementCents  int64
	auctionID       string
}

func main() {
	cfg := parseFlags()

	// The default http.Transport caps idle connections per host at 2,
	// which is fine for normal traffic but starves a deliberate burst of
	// hundreds of simultaneous new connections to the same host: some
	// requests fail at the transport layer before ever reaching the
	// server. Raising the pool ceilings is what makes "fire N requests at
	// once" actually mean that at the TCP layer, not just in application code.
	transport := &http.Transport{
		MaxIdleConns:        cfg.concurrency + 50,
		MaxIdleConnsPerHost: cfg.concurrency + 50,
		MaxConnsPerHost:     0, // unlimited
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: transport}

	fmt.Printf("BidCraft stress tool -- base=%s mode=%s concurrency=%d\n", cfg.base, cfg.mode, cfg.concurrency)

	tokens, err := loginOrRegisterUsers(client, cfg)
	must(err, "preparing users")
	fmt.Printf("Authenticated %d users\n", len(tokens))

	auctionID := cfg.auctionID
	var basePriceCents int64 = 1000
	if auctionID == "" {
		auctionID, basePriceCents, err = createAuction(client, cfg, tokens[0])
		must(err, "creating auction")
		fmt.Printf("Created auction %s (base price %d cents)\n", auctionID, basePriceCents)
	}

	results := fireBids(client, cfg, tokens, auctionID, basePriceCents)

	ok := verify(client, cfg, auctionID, results)

	printReport(results)
	if ok {
		fmt.Println("\nPASS")
		os.Exit(0)
	}
	fmt.Println("\nFAIL")
	os.Exit(1)
}

func parseFlags() config {
	var c config
	flag.StringVar(&c.base, "base", "http://localhost:8080", "backend base URL")
	flag.IntVar(&c.users, "users", 20, "number of distinct bidders")
	flag.IntVar(&c.concurrency, "concurrency", 500, "number of simultaneous bid requests")
	flag.StringVar(&c.mode, "mode", "burst", "burst | tie")
	flag.IntVar(&c.durationSeconds, "duration-seconds", 30, "duration of the auction created for the test")
	flag.Int64Var(&c.incrementCents, "increment-cents", 100, "minimum increment for the created auction")
	flag.StringVar(&c.auctionID, "auction", "", "bid against an existing auction instead of creating one")
	flag.Parse()
	return c
}

// --- wire-format structs (mirrors the documented API contract) ---

type authResp struct {
	Token string `json:"token"`
	User  struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
}

type auctionResp struct {
	ID                string `json:"id"`
	BasePriceCents    int64  `json:"base_price_cents"`
	CurrentPriceCents int64  `json:"current_price_cents"`
}

type bidsListResp struct {
	Data []struct {
		Seq         int64  `json:"seq"`
		AmountCents int64  `json:"amount_cents"`
		BidderID    string `json:"bidder_id"`
	} `json:"data"`
}

type errorResp struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func loginOrRegisterUsers(client *http.Client, cfg config) ([]string, error) {
	seeded := []string{"alice@bidcraft.dev", "bob@bidcraft.dev", "carol@bidcraft.dev"}
	var tokens []string

	for i := 0; i < cfg.users; i++ {
		var email, username string
		if i < len(seeded) {
			email = seeded[i]
			username = seeded[i][:len(seeded[i])-len("@bidcraft.dev")]
		} else {
			suffix := uuid.NewString()[:8]
			username = fmt.Sprintf("stress_%s", suffix)
			email = fmt.Sprintf("%s@bidcraft.dev", username)
			if err := registerUser(client, cfg.base, email, username); err != nil {
				return nil, fmt.Errorf("register %s: %w", email, err)
			}
		}

		token, err := loginUser(client, cfg.base, email, "demo1234")
		if err != nil {
			return nil, fmt.Errorf("login %s: %w", email, err)
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func registerUser(client *http.Client, base, email, username string) error {
	body, _ := json.Marshal(map[string]string{"email": email, "username": username, "password": "demo1234"})
	resp, err := client.Post(base+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}
	return nil
}

func loginUser(client *http.Client, base, email, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	resp, err := client.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}
	var ar authResp
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return "", err
	}
	return ar.Token, nil
}

func createAuction(client *http.Client, cfg config, token string) (string, int64, error) {
	base := int64(1000)
	payload := map[string]any{
		"title":               "Stress Test Auction",
		"description":         "created by cmd/stress",
		"image_url":           "https://example.com/stress.png",
		"base_price_cents":    base,
		"min_increment_cents": cfg.incrementCents,
		"duration_seconds":    cfg.durationSeconds,
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, cfg.base+"/api/v1/auctions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}
	var a auctionResp
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return "", 0, err
	}
	return a.ID, a.BasePriceCents, nil
}

type bidResult struct {
	status    int
	errorCode string
	latency   time.Duration
	is5xx     bool
}

// fireBids spawns cfg.concurrency goroutines, each with its request
// pre-built, all released by a single close(start) -- a true barrier, so
// the burst is genuinely simultaneous rather than a staggered ramp.
func fireBids(client *http.Client, cfg config, tokens []string, auctionID string, basePriceCents int64) []bidResult {
	start := make(chan struct{})
	results := make([]bidResult, cfg.concurrency)
	var wg sync.WaitGroup

	for i := 0; i < cfg.concurrency; i++ {
		i := i
		token := tokens[i%len(tokens)]

		var amount int64
		switch cfg.mode {
		case "tie":
			amount = basePriceCents // every goroutine bids the identical amount
		default: // burst
			amount = basePriceCents + int64(i)*cfg.incrementCents
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			t0 := time.Now()
			status, code, err := placeBid(client, cfg.base, auctionID, token, amount)
			latency := time.Since(t0)

			results[i] = bidResult{
				status:    status,
				errorCode: code,
				latency:   latency,
				is5xx:     status >= 500 || err != nil,
			}
		}()
	}

	close(start)
	wg.Wait()
	return results
}

func placeBid(client *http.Client, base, auctionID, token string, amountCents int64) (int, string, error) {
	body, _ := json.Marshal(map[string]int64{"amount_cents": amountCents})
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/auctions/"+auctionID+"/bids", bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return 0, "TRANSPORT_ERROR", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated {
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, "", nil
	}
	var er errorResp
	json.NewDecoder(resp.Body).Decode(&er)
	return resp.StatusCode, er.Error.Code, nil
}

// verify checks the empirical outcome against the live API -- not just
// local counters -- so the tool proves the SERVER's state is consistent,
// not merely that this process's bookkeeping adds up.
func verify(client *http.Client, cfg config, auctionID string, results []bidResult) bool {
	accepted := 0
	for _, r := range results {
		if r.status == http.StatusCreated {
			accepted++
		}
	}

	resp, err := client.Get(cfg.base + "/api/v1/auctions/" + auctionID + "/bids?limit=500")
	if err != nil {
		fmt.Println("VERIFY FAILED: could not fetch bid history:", err)
		return false
	}
	defer resp.Body.Close()
	var history bidsListResp
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		fmt.Println("VERIFY FAILED: could not decode bid history:", err)
		return false
	}

	ok := true

	if len(history.Data) != accepted {
		fmt.Printf("VERIFY FAILED: accepted(201)=%d but history has %d rows\n", accepted, len(history.Data))
		ok = false
	}

	for i := 1; i < len(history.Data); i++ {
		if history.Data[i].Seq <= history.Data[i-1].Seq {
			fmt.Println("VERIFY FAILED: history is not strictly increasing by seq")
			ok = false
			break
		}
		if history.Data[i].AmountCents <= history.Data[i-1].AmountCents {
			fmt.Println("VERIFY FAILED: history amounts are not strictly increasing")
			ok = false
			break
		}
	}

	seen := map[int64]bool{}
	for _, b := range history.Data {
		if seen[b.AmountCents] {
			fmt.Printf("VERIFY FAILED: duplicate accepted amount %d in history\n", b.AmountCents)
			ok = false
		}
		seen[b.AmountCents] = true
	}

	if cfg.mode == "tie" && accepted != 1 {
		fmt.Printf("VERIFY FAILED: tie mode expected exactly 1 accepted bid, got %d\n", accepted)
		ok = false
	}

	aResp, err := client.Get(cfg.base + "/api/v1/auctions/" + auctionID)
	if err == nil {
		defer aResp.Body.Close()
		var a auctionResp
		if json.NewDecoder(aResp.Body).Decode(&a) == nil && len(history.Data) > 0 {
			maxAmount := history.Data[len(history.Data)-1].AmountCents
			if a.CurrentPriceCents != maxAmount {
				fmt.Printf("VERIFY FAILED: auction.current_price_cents=%d != max accepted amount=%d\n", a.CurrentPriceCents, maxAmount)
				ok = false
			}
		}
	}

	for _, r := range results {
		if r.is5xx {
			fmt.Println("VERIFY FAILED: at least one 5xx or transport error occurred")
			ok = false
			break
		}
	}

	return ok
}

func printReport(results []bidResult) {
	var latencies []time.Duration
	accepted, rejected := 0, 0
	byCode := map[string]int{}

	for _, r := range results {
		latencies = append(latencies, r.latency)
		if r.status == http.StatusCreated {
			accepted++
		} else {
			rejected++
			byCode[r.errorCode]++
		}
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	pct := func(p float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(float64(len(latencies)-1) * p)
		return latencies[idx]
	}

	fmt.Printf("\nAccepted: %d  Rejected: %d\n", accepted, rejected)
	fmt.Println("Rejection breakdown:")
	for code, n := range byCode {
		fmt.Printf("  %-24s %d\n", code, n)
	}
	fmt.Printf("Latency p50=%v p95=%v p99=%v\n", pct(0.50), pct(0.95), pct(0.99))
}

func must(err error, ctx string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s: %v\n", ctx, err)
		os.Exit(1)
	}
}
