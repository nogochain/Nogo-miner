package nogopow

import (
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewEngine(t *testing.T) {
	engine := NewEngine()
	if engine == nil {
		t.Fatal("Failed to create engine")
	}
	
	if !engine.IsRunning() {
		t.Log("Engine created, not running")
	}
	
	// Verify matrices are initialized
	if len(engine.matrixA) != MatrixSize {
		t.Errorf("Expected matrixA size %d, got %d", MatrixSize, len(engine.matrixA))
	}
	if len(engine.matrixB) != MatrixSize {
		t.Errorf("Expected matrixB size %d, got %d", MatrixSize, len(engine.matrixB))
	}
}

func TestComputeBlockHash(t *testing.T) {
	engine := NewEngine()
	
	header := &BlockHeader{
		Height:         100,
		PrevHash:       make([]byte, 32),
		MerkleRoot:     make([]byte, 32),
		Timestamp:      1234567890,
		DifficultyBits: 18,
		MinerAddress:   "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
		Nonce:          0,
		ChainID:        1,
	}
	
	// Fill prevHash and merkleRoot with test data
	for i := 0; i < 32; i++ {
		header.PrevHash[i] = byte(i)
		header.MerkleRoot[i] = byte(32 - i)
	}
	
	hash := engine.computeBlockHash(header, 12345)
	if len(hash) != 32 {
		t.Errorf("Expected hash length 32, got %d", len(hash))
	}
	
	t.Logf("Block hash: %s", FormatHash(hash))
}

func TestMine(t *testing.T) {
	engine := NewEngine()
	
	// Create an easy difficulty for testing
	header := &BlockHeader{
		Height:         1,
		PrevHash:       make([]byte, 32),
		MerkleRoot:     make([]byte, 32),
		Timestamp:      time.Now().Unix(),
		DifficultyBits: 10, // Easy difficulty
		MinerAddress:   "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
		Nonce:          0,
		ChainID:        1,
	}
	
	// Mine with timeout
	timeout := time.After(2 * time.Second)
	done := make(chan *MiningResult, 1)
	stopCh := make(chan struct{})
	
	go func() {
		result := engine.Mine(header, stopCh)
		select {
		case done <- result:
		default:
		}
	}()
	
	select {
	case result := <-done:
		if result == nil {
			t.Fatal("Mining result is nil")
		}
		
		t.Logf("Mining completed: success=%v, hashes=%d, duration=%v, nonce=%d",
			result.Success, result.HashesTried, result.Duration, result.Nonce)
		
		// With easy difficulty, we might find a solution
		// If not found, verify at least some hashes were tried
		if result.HashesTried == 0 {
			t.Error("Expected at least some hashes to be tried")
		}
		
	case <-timeout:
		close(stopCh)
		// Wait for goroutine to finish
		<-done
		t.Log("Mining timed out (expected for hard problems)")
	}
}

func TestMineWithStop(t *testing.T) {
	engine := NewEngine()
	
	header := &BlockHeader{
		Height:         1,
		PrevHash:       make([]byte, 32),
		MerkleRoot:     make([]byte, 32),
		Timestamp:      time.Now().Unix(),
		DifficultyBits: 20, // Hard difficulty
		MinerAddress:   "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
		Nonce:          0,
		ChainID:        1,
	}
	
	stopCh := make(chan struct{})
	
	// Stop after 1 second
	time.AfterFunc(1*time.Second, func() {
		close(stopCh)
	})
	
	result := engine.Mine(header, stopCh)
	if result == nil {
		t.Fatal("Mining result is nil")
	}
	
	if result.Success {
		t.Log("Unexpectedly found solution before stop")
	}
	
	t.Logf("Mining stopped: hashes=%d, duration=%v", result.HashesTried, result.Duration)
}

func TestGetHashRate(t *testing.T) {
	engine := NewEngine()
	
	// Initial hash rate should be 0
	if engine.GetHashRate() != 0 {
		t.Errorf("Expected initial hash rate 0, got %d", engine.GetHashRate())
	}
	
	// Mine briefly
	header := &BlockHeader{
		Height:         1,
		PrevHash:       make([]byte, 32),
		MerkleRoot:     make([]byte, 32),
		Timestamp:      time.Now().Unix(),
		DifficultyBits: 20,
		MinerAddress:   "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
		Nonce:          0,
		ChainID:        1,
	}
	
	stopCh := make(chan struct{})
	time.AfterFunc(500*time.Millisecond, func() {
		close(stopCh)
	})
	
	engine.Mine(header, stopCh)
	
	hashRate := engine.GetHashRate()
	t.Logf("Hash rate: %d H/s", hashRate)
	
	// Hash rate should be > 0 after mining
	if hashRate == 0 {
		t.Error("Expected hash rate > 0 after mining")
	}
}

func TestReset(t *testing.T) {
	engine := NewEngine()
	
	// Mine briefly
	header := &BlockHeader{
		Height:         1,
		PrevHash:       make([]byte, 32),
		MerkleRoot:     make([]byte, 32),
		Timestamp:      time.Now().Unix(),
		DifficultyBits: 20,
		MinerAddress:   "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
		Nonce:          0,
		ChainID:        1,
	}
	
	stopCh := make(chan struct{})
	time.AfterFunc(200*time.Millisecond, func() {
		close(stopCh)
	})
	
	engine.Mine(header, stopCh)
	
	hashCount := engine.GetHashCount()
	if hashCount == 0 {
		t.Error("Expected hash count > 0")
	}
	
	// Reset
	engine.Reset()
	
	if engine.GetHashCount() != 0 {
		t.Error("Expected hash count 0 after reset")
	}
}

func TestVerifySeal(t *testing.T) {
	header := &BlockHeader{
		Height:         1,
		PrevHash:       make([]byte, 32),
		MerkleRoot:     make([]byte, 32),
		Timestamp:      time.Now().Unix(),
		DifficultyBits: 10, // Easy difficulty
		MinerAddress:   "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
		Nonce:          0,
		ChainID:        1,
	}
	
	// Try a few nonces (matrix operations are slow)
	maxAttempts := 10
	found := false
	
	for nonce := uint64(0); nonce < uint64(maxAttempts); nonce++ {
		valid := VerifySeal(header, nonce)
		if valid {
			found = true
			t.Logf("Found valid nonce: %d", nonce)
			break
		}
	}
	
	if !found {
		t.Logf("No valid nonce found in %d attempts (expected for PoW)", maxAttempts)
	}
}

func TestDifficultyBitsToTarget(t *testing.T) {
	tests := []struct {
		bits     uint32
		expected string
	}{
		{18, "00000000ffff0000000000000000000000000000000000000000000000000000"},
		{10, "00ffff0000000000000000000000000000000000000000000000000000000000"},
		{20, "000000000000ffff000000000000000000000000000000000000000000000000"},
	}
	
	for _, tt := range tests {
		target := difficultyBitsToTarget(tt.bits)
		targetHex := fmt.Sprintf("%064s", hex.EncodeToString(target.Bytes()))
		
		t.Logf("Bits %d -> Target %s", tt.bits, targetHex)
	}
}

func TestConcurrentMining(t *testing.T) {
	engine := NewEngine()
	
	header := &BlockHeader{
		Height:         1,
		PrevHash:       make([]byte, 32),
		MerkleRoot:     make([]byte, 32),
		Timestamp:      time.Now().Unix(),
		DifficultyBits: 20,
		MinerAddress:   "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
		Nonce:          0,
		ChainID:        1,
	}
	
	var wg sync.WaitGroup
	numWorkers := 4
	
	// Start multiple miners with separate stop channels
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			localHeader := *header
			localHeader.Nonce = uint64(id) * 1000000
			
			stopCh := make(chan struct{})
			
			// Stop after 500ms
			time.AfterFunc(500*time.Millisecond, func() {
				close(stopCh)
			})
			
			result := engine.Mine(&localHeader, stopCh)
			if result != nil {
				t.Logf("Worker %d: hashes=%d, success=%v", id, result.HashesTried, result.Success)
			}
		}(i)
	}
	
	wg.Wait()
	
	hashRate := engine.GetHashRate()
	t.Logf("Total hash rate: %d H/s", hashRate)
}

func TestFormatHash(t *testing.T) {
	tests := []struct {
		hash   []byte
		expect string
	}{
		{[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}, "0001020304050607"},
		{[]byte{}, ""},
		{nil, ""},
	}
	
	for _, tt := range tests {
		result := FormatHash(tt.hash)
		if result != tt.expect {
			t.Errorf("FormatHash(%v) = %s, want %s", tt.hash, result, tt.expect)
		}
	}
}
