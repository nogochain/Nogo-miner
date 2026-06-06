// Package nogopow implements the NogoPow proof-of-work algorithm
// This implementation matches the node's algorithm exactly
package nogopow

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/crypto/sha3"
)

// Constants - EXACT copy from node
const (
	matSize          = 256
	matNum           = 256 // MUST match node!
	FixedPointFactor = 1 << 24
	FixedPointHalf   = 1 << 23
	FixedPointShift  = 24
	MaxNonce         = 1000000000
)

// BlockHeader represents a block header for mining
type BlockHeader struct {
	Height       uint64
	PrevHash     []byte
	MerkleRoot   []byte
	StateRoot    []byte // State root hash for PoW calculation (World State MPT root)
	Timestamp    int64
	Difficulty   *big.Int
	MinerAddress []byte
	ChainID      uint64
}

// MiningResult represents the result of a mining attempt
type MiningResult struct {
	Nonce       uint64
	BlockHash   []byte
	Success     bool
	HashesTried uint64
	Duration    time.Duration
}

// Engine represents the NogoPow mining engine
type Engine struct {
	mu        sync.RWMutex
	running   bool
	hashCount uint64
	startTime time.Time
	cache     *Cache
}

// NewEngine creates a new NogoPow engine
func NewEngine() *Engine {
	return &Engine{
		cache: NewCache(),
	}
}

// Mine performs mining on the given block header
func (e *Engine) Mine(header *BlockHeader, stopCh <-chan struct{}) *MiningResult {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return nil
	}
	e.running = true
	e.startTime = time.Now()
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
	}()

	// Calculate target from difficulty
	target := difficultyToTarget(header.Difficulty)

	var localHashCount uint64 = 0
	startTime := time.Now()

	// Calculate seed from parent hash (same as node)
	seed := calcSeed(header.PrevHash)

	// Get cache data for this seed (used in computePOW)
	_ = e.cache.GetData(seed[:])

	// Start from a random nonce to avoid all workers submitting nonce=0
	startNonce := uint64(time.Now().UnixNano() % 1000000)

	// Try nonces until we find a valid one or are stopped
	for nonce := startNonce; nonce < MaxNonce; nonce++ {
		select {
		case <-stopCh:
			return &MiningResult{
				Nonce:       nonce,
				BlockHash:   nil,
				Success:     false,
				HashesTried: localHashCount,
				Duration:    time.Since(startTime),
			}
		default:
		}

		// Compute block hash with this nonce using node's algorithm
		blockHash := e.computeBlockHash(header, nonce)

		localHashCount++

		// Check if hash meets target
		hashBig := new(big.Int).SetBytes(blockHash)
		if hashBig.Cmp(target) <= 0 {
			// Found valid hash!
			// Log comprehensive debug info for cross-referencing with pool
			fmt.Printf("[NogoPow] ✅ Solution found: nonce=%d, hashes=%d, duration=%v\n",
				nonce, localHashCount, time.Since(startTime))
			fmt.Printf("[NogoPow]   Header: height=%d, prevHash=%x, merkleRoot=%x, timestamp=%d, difficulty=%s, minerAddr=%x\n",
				header.Height, header.PrevHash[:min(8, len(header.PrevHash))],
				header.MerkleRoot[:min(8, len(header.MerkleRoot))],
				header.Timestamp, header.Difficulty.String(),
				header.MinerAddress[:min(8, len(header.MinerAddress))])
			fmt.Printf("[NogoPow]   Result: hash=%x, target=%s\n", blockHash, target.String())

			return &MiningResult{
				Nonce:       nonce,
				BlockHash:   blockHash,
				Success:     true,
				HashesTried: localHashCount,
				Duration:    time.Since(startTime),
			}
		}

		// Update hash counter
		e.mu.Lock()
		e.hashCount++
		e.mu.Unlock()
	}

	// Exhausted all nonces without finding solution
	return &MiningResult{
		Nonce:       MaxNonce,
		BlockHash:   nil,
		Success:     false,
		HashesTried: localHashCount,
		Duration:    time.Since(startTime),
	}
}

// computeBlockHash computes the hash using node's exact algorithm
// CRITICAL: Must include Coinbase and Root in the header to match
// the node/pool rlpEncode serialization. Omitting these fields causes
// sealHash mismatch and perpetual "invalid_pow" rejection.
func (e *Engine) computeBlockHash(header *BlockHeader, nonce uint64) []byte {
	// Decode miner address from NOGO-format hex string to 20-byte Address
	// The pool uses stringToAddress which strips "NOGO" prefix, hex-decodes,
	// and takes the first 20 bytes. Must produce identical bytes.
	coinbase := decodeMinerAddress(header.MinerAddress)

	// Convert merkleRoot bytes to 32-byte Hash
	// The pool uses copy(merkleRootHash[:], merkleRootBytes)
	root := BytesToHash(header.MerkleRoot)

	// Create header with ALL fields matching the pool/node rlpEncode order
	// CRITICAL: Root = state root, TxHash = tx root
	var stateRootHash Hash
	if len(header.StateRoot) > 0 {
		copy(stateRootHash[:], header.StateRoot)
	}
	blockHeader := &Header{
		ParentHash: BytesToHash(header.PrevHash),
		Coinbase:   coinbase,
		Root:       stateRootHash,       // State root (World State MPT root)
		TxHash:     root,                // Transactions root (Merkle tree root)
		Number:     new(big.Int).SetUint64(header.Height),
		Time:       uint64(header.Timestamp),
		Nonce:      uint64ToBlockNonce(nonce),
		Difficulty: header.Difficulty,
	}

	// Compute seal hash (RLP + Keccak-256) - must match node/pool
	blockHash := e.sealHash(blockHeader)

	// Compute PoW using cache - must match node/pool
	powHash := e.computePoW(blockHash, seedFromParent(header.PrevHash))

	return powHash[:]
}

// sealHash computes the hash of a block header prior to sealing
func (e *Engine) sealHash(header *Header) Hash {
	hasher := sha3.NewLegacyKeccak256()
	rlpEncode(hasher, header)
	return BytesToHash(hasher.Sum(nil))
}

// computePoW computes the proof-of-work hash using NogoPow algorithm
func (e *Engine) computePoW(blockHash, seed Hash) Hash {
	cacheData := e.cache.GetData(seed.Bytes())
	result := mulMatrix(blockHash.Bytes(), cacheData)
	return hashMatrix(result)
}

// difficultyToTarget converts difficulty to target threshold
func difficultyToTarget(difficulty *big.Int) *big.Int {
	maxTarget := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	target := new(big.Int).Div(maxTarget, difficulty)
	return target
}

// uint64ToBlockNonce converts uint64 nonce to BlockNonce
func uint64ToBlockNonce(nonce uint64) BlockNonce {
	var n BlockNonce
	binary.LittleEndian.PutUint64(n[:8], nonce)
	return n
}

// calcSeed calculates the seed hash from parent block
func calcSeed(prevHash []byte) Hash {
	if len(prevHash) == 0 {
		return Hash{}
	}
	return BytesToHash(prevHash)
}

// seedFromParent converts parent hash to seed
func seedFromParent(prevHash []byte) Hash {
	return BytesToHash(prevHash)
}

// decodeMinerAddress decodes a miner address from NOGO-format hex string to 20-byte Address.
// The miner receives the address as raw ASCII bytes from the Stratum job (e.g., "NOGO00ec...").
// This function strips the "NOGO" prefix, hex-decodes, and extracts the first 20 bytes,
// matching the pool's stringToAddress implementation exactly.
func decodeMinerAddress(addrBytes []byte) Address {
	var result Address

	if len(addrBytes) == 0 {
		return result
	}

	addrStr := string(addrBytes)

	// Strip "NOGO" prefix if present (matches pool stringToAddress)
	if len(addrStr) >= 4 && addrStr[:4] == "NOGO" {
		addrStr = addrStr[4:]
	}

	// Hex-decode the remaining string
	decoded, err := hex.DecodeString(addrStr)
	if err != nil || len(decoded) < 20 {
		// Return zero address on decode failure — the resulting sealHash
		// will not match pool verification, which is the correct behavior
		// for invalid miner addresses
		return result
	}

	copy(result[:], decoded[:20])
	return result
}

// Header represents a block header (matches node structure)
type Header struct {
	ParentHash Hash
	Coinbase   Address
	Root       Hash
	TxHash     Hash
	Number     *big.Int
	GasLimit   uint64
	Time       uint64
	Extra      []byte
	Nonce      BlockNonce
	Difficulty *big.Int
}

// Address represents a 20-byte address
type Address [20]byte

// Bytes returns address bytes
func (a Address) Bytes() []byte { return a[:] }

// Hash represents a 32-byte hash
type Hash [32]byte

// Bytes returns hash as byte slice
func (h Hash) Bytes() []byte { return h[:] }

// Hex returns hex string representation
func (h Hash) Hex() string {
	return fmt.Sprintf("%x", h[:])
}

// BlockNonce represents a 32-byte nonce
type BlockNonce [32]byte

// rlpEncode encodes header fields sequentially (matches node)
func rlpEncode(w interface{}, v interface{}) {
	header, ok := v.(*Header)
	if !ok {
		return
	}

	writer, ok := w.(interface{ Write([]byte) (int, error) })
	if !ok {
		return
	}

	// Encode each field in the same order as node
	writer.Write(header.ParentHash.Bytes())
	writer.Write(header.Coinbase.Bytes())
	writer.Write(header.Root.Bytes())
	writer.Write(header.TxHash.Bytes())

	// Number as big.Int bytes
	if header.Number != nil {
		writer.Write(header.Number.Bytes())
	}

	// GasLimit as 8 bytes
	gasBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(gasBytes, header.GasLimit)
	writer.Write(gasBytes)

	// Time as 8 bytes
	timeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timeBytes, header.Time)
	writer.Write(timeBytes)

	// Extra data
	if len(header.Extra) > 0 {
		writer.Write(header.Extra)
	}

	// Nonce
	writer.Write(header.Nonce[:])

	// Difficulty as big.Int bytes
	if header.Difficulty != nil {
		writer.Write(header.Difficulty.Bytes())
	}
}

// BytesToHash converts bytes to hash
func BytesToHash(b []byte) Hash {
	var h Hash
	if len(b) > 32 {
		b = b[len(b)-32:]
	}
	copy(h[32-len(b):], b)
	return h
}

// GetHashRate returns current hash rate in hashes per second
func (e *Engine) GetHashRate() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	elapsed := time.Since(e.startTime)
	if elapsed == 0 {
		return 0
	}

	return uint64(float64(e.hashCount) / elapsed.Seconds())
}

// Stop stops the mining engine
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.running = false
}

// mulMatrix performs matrix multiplication (matches node mulMatrixPooled)
func mulMatrix(headerHash []byte, cache []uint32) []uint8 {
	ui32data := make([]uint32, matNum*matSize*matSize/4)

	for i := 0; i < 128; i++ {
		start := i * 1024 * 32
		for j := 0; j < 512; j++ {
			copy(ui32data[start+j*32:start+j*32+32], cache[start+j*64:start+j*64+32])
			copy(ui32data[start+512*32+j*32:start+512*32+j*32+32], cache[start+j*64+32:start+j*64+64])
		}
	}

	byteData := make([]byte, len(ui32data)*4)
	for i, v := range ui32data {
		binary.LittleEndian.PutUint32(byteData[i*4:i*4+4], v)
	}

	fixedData := make([]int64, matNum*matSize*matSize)
	for i := 0; i < matNum*matSize*matSize; i++ {
		fixedData[i] = toFixed(float64(int8(byteData[i])))
	}

	dataIdentity := make([]int64, matSize*matSize)
	for i := 0; i < matSize; i++ {
		dataIdentity[i*257] = FixedPointFactor
	}

	var tmp [matSize][matSize]int64
	var maArr [4][matSize][matSize]int64

	var wg sync.WaitGroup
	wg.Add(4)

	for k := 0; k < 4; k++ {
		go func(i int) {
			defer wg.Done()

			localMatA := newDenseMatrix(matSize, matSize, nil)
			localMatB := newDenseMatrix(matSize, matSize, nil)

			copy(localMatA.data, dataIdentity)

			var sequence [32]byte
			hasher := sha3.NewLegacyKeccak256()
			hasher.Write(headerHash[i*8 : (i+1)*8])
			copy(sequence[:], hasher.Sum(nil))

			for j := 0; j < 2; j++ {
				for k := 0; k < 32; k++ {
					index := int(sequence[k])
					mb := newDenseMatrix(matSize, matSize, fixedData[index*matSize*matSize:(index+1)*matSize*matSize])

					mulMatrixBlocked(localMatB.data, localMatA.data, mb.data, matSize)

					for row := 0; row < matSize; row++ {
						for col := 0; col < matSize; col++ {
							i8v := fromFixed(localMatB.At(row, col))
							localMatB.Set(row, col, toFixedShift(i8v))
						}
					}
					localMatA, localMatB = localMatB, localMatA
				}
			}

			for row := 0; row < matSize; row++ {
				for col := 0; col < matSize; col++ {
					maArr[i][row][col] = localMatA.At(row, col)
				}
			}
		}(k)
	}
	wg.Wait()

	for i := 0; i < 4; i++ {
		for row := 0; row < matSize; row++ {
			for col := 0; col < matSize; col++ {
				tmp[row][col] += maArr[i][row][col]
			}
		}
	}

	result := make([]uint8, 0, matSize*matSize)
	for i := 0; i < matSize; i++ {
		for j := 0; j < matSize; j++ {
			result = append(result, uint8(fromFixed(tmp[i][j])))
		}
	}
	return result
}

// hashMatrix computes the final hash from matrix result
func hashMatrix(result []uint8) [32]byte {
	var mat8 [matSize][matSize]uint8
	for i := 0; i < matSize; i++ {
		for j := 0; j < matSize; j++ {
			mat8[i][j] = result[i*matSize+j]
		}
	}

	var mat32 [matSize][matSize / 4]uint32

	for i := 0; i < matSize; i++ {
		for j := 0; j < matSize/4; j++ {
			mat32[i][j] = (uint32(mat8[i][j+192]) << 24) |
				(uint32(mat8[i][j+128]) << 16) |
				(uint32(mat8[i][j+64]) << 8) |
				(uint32(mat8[i][j]) << 0)
		}
	}

	for k := matSize; k > 1; k = k / 2 {
		for j := 0; j < k/2; j++ {
			for i := 0; i < matSize/4; i++ {
				mat32[j][i] = fnv(mat32[j][i], mat32[j+k/2][i])
			}
		}
	}

	ui32data := make([]uint32, 0, matSize/4)
	for i := 0; i < matSize/4; i++ {
		ui32data = append(ui32data, mat32[0][i])
	}

	// Security: Use binary.LittleEndian for safe type conversion
	dataBytes := make([]byte, len(ui32data)*4)
	for i, v := range ui32data {
		binary.LittleEndian.PutUint32(dataBytes[i*4:i*4+4], v)
	}

	var h [32]byte
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(dataBytes)
	copy(h[:], hasher.Sum(nil))

	return h
}

// toFixed converts float64 to fixed point
func toFixed(val float64) int64 {
	return int64(val * FixedPointFactor)
}

// fromFixed converts fixed point to int8
func fromFixed(val int64) int8 {
	rounded := (val + FixedPointHalf) >> FixedPointShift
	if rounded > 127 {
		return 127
	}
	if rounded < -128 {
		return -128
	}
	return int8(rounded)
}

// toFixedShift converts int8 to fixed point
func toFixedShift(v int8) int64 {
	return int64(v) << FixedPointShift
}

// mulMatrixBlocked performs blocked matrix multiplication (EXACT copy from node)
func mulMatrixBlocked(dst, a, b []int64, size int) {
	const blockSize = 32

	// Initialize destination to zero
	for i := 0; i < size*size; i++ {
		dst[i] = 0
	}

	for i0 := 0; i0 < size; i0 += blockSize {
		i1 := i0 + blockSize
		if i1 > size {
			i1 = size
		}

		for k0 := 0; k0 < size; k0 += blockSize {
			k1 := k0 + blockSize
			if k1 > size {
				k1 = size
			}

			for j0 := 0; j0 < size; j0 += blockSize {
				j1 := j0 + blockSize
				if j1 > size {
					j1 = size
				}

				for i := i0; i < i1; i++ {
					rowA := i * size
					rowDst := i * size

					for k := k0; k < k1; k++ {
						valA := a[rowA+k]

						if valA == 0 {
							continue
						}

						rowB := k * size

						for j := j0; j < j1; j++ {
							prod := valA * b[rowB+j]
							dst[rowDst+j] += (prod + FixedPointHalf) >> FixedPointShift
						}
					}
				}
			}
		}
	}
}

// newDenseMatrix creates a new dense matrix
func newDenseMatrix(rows, cols int, data []int64) *denseMatrix {
	if data == nil {
		data = make([]int64, rows*cols)
		for i := 0; i < rows; i++ {
			data[i*cols+i] = FixedPointFactor
		}
	}
	return &denseMatrix{
		data: data,
		rows: rows,
		cols: cols,
	}
}

// denseMatrix represents a dense matrix
type denseMatrix struct {
	data []int64
	rows int
	cols int
}

// At returns the value at row, col
func (m *denseMatrix) At(row, col int) int64 {
	return m.data[row*m.cols+col]
}

// Set sets the value at row, col
func (m *denseMatrix) Set(row, col int, v int64) {
	m.data[row*m.cols+col] = v
}

// Reset resets the matrix dimensions
func (m *denseMatrix) Reset(rows, cols int) {
	if rows > m.rows || cols > m.cols {
		m.data = make([]int64, rows*cols)
	} else {
		clear(m.data[:rows*cols])
	}
	m.rows = rows
	m.cols = cols
}

// GetMatrix gets a matrix from the pool
func GetMatrix(rows, cols int) *denseMatrix {
	m := &denseMatrix{
		data: make([]int64, rows*cols),
		rows: rows,
		cols: cols,
	}
	for i := 0; i < rows; i++ {
		m.data[i*cols+i] = FixedPointFactor
	}
	return m
}

// PutMatrix puts a matrix back to the pool
func PutMatrix(m *denseMatrix) {
	// In production, would use sync.Pool
}

// fnv computes FNV hash (matches node implementation)
func fnv(a, b uint32) uint32 {
	// CRITICAL: Must match node implementation exactly!
	// Node uses: a * prime ^ b (not standard FNV-1a)
	return a*0x01000193 ^ b
}

const maxCacheItems = 64

// Cache represents the LRU cache for NogoPow
type Cache struct {
	data  map[string][]uint32
	order []string
	lock  sync.RWMutex
}

// NewCache creates a new cache
func NewCache() *Cache {
	return &Cache{
		data:  make(map[string][]uint32),
		order: make([]string, 0, maxCacheItems),
	}
}

// GetData gets cache data for seed with LRU eviction
func (c *Cache) GetData(seed []byte) []uint32 {
	seedStr := string(seed)

	c.lock.RLock()
	if data, ok := c.data[seedStr]; ok {
		c.lock.RUnlock()
		return data
	}
	c.lock.RUnlock()

	c.lock.Lock()
	defer c.lock.Unlock()

	if data, ok := c.data[seedStr]; ok {
		return data
	}

	if len(c.data) >= maxCacheItems {
		oldest := c.order[0]
		delete(c.data, oldest)
		c.order = c.order[1:]
	}

	data := calcSeedCache(seed)
	c.data[seedStr] = data
	c.order = append(c.order, seedStr)
	return data
}

// calcSeedCache calculates cache data from seed
func calcSeedCache(seed []byte) []uint32 {
	extSeed := extendBytes(seed, 3)
	// v should be N * 16 * 2 * r = 1024 * 32 = 32768 uint32
	v := make([]uint32, 32*1024)

	if !isLittleEndian() {
		swap(extSeed)
	}

	// cache should be 128 * v = 128 * 32768 = 4,194,304 uint32
	cache := make([]uint32, 128*32*1024)
	for i := 0; i < 128; i++ {
		Smix(extSeed, v)
		copy(cache[i*32*1024:], v)
	}

	return cache
}

// extendBytes extends seed bytes
func extendBytes(seed []byte, round int) []byte {
	extSeed := make([]byte, len(seed)*(round+1))
	copy(extSeed, seed)

	for i := 0; i < round; i++ {
		var h [32]byte
		hasher := sha3.NewLegacyKeccak256()
		start := i * 32
		hasher.Write(extSeed[start : start+32])
		copy(h[:], hasher.Sum(nil))
		copy(extSeed[(i+1)*32:(i+2)*32], h[:])
	}

	return extSeed
}

// isLittleEndian detects the system's byte order.
// Uses unsafe pointer to read the raw memory representation of a uint32.
// On little-endian: 0x01020304 stored as [0x04, 0x03, 0x02, 0x01], first byte == 0x04
// On big-endian:    0x01020304 stored as [0x01, 0x02, 0x03, 0x04], first byte == 0x01
func isLittleEndian() bool {
	n := uint32(0x01020304)
	return *(*byte)(unsafe.Pointer(&n)) == 0x04
}

// swap swaps byte order for big-endian systems
func swap(buffer []byte) {
	for i := 0; i+4 <= len(buffer); i += 4 {
		binary.LittleEndian.PutUint32(buffer[i:], binary.BigEndian.Uint32(buffer[i:]))
	}
}

// Smix performs the Smix operation (scrypt ROMix)
func Smix(b []byte, v []uint32) {
	const r = 1
	const N = 1024

	x := make([]uint32, 16*2*r)
	// Unmarshal b into x
	for i := 0; i < 16*2*r; i++ {
		x[i] = binary.LittleEndian.Uint32(b[i*4:])
	}

	// Initialize v and compute x
	for i := 0; i < N; i++ {
		copy(v[i*16*2*r:], x)
		x = blockMix(x, r)
	}

	// Compute final x
	for i := 0; i < N; i++ {
		j := int(x[16*(2*r-1)] % uint32(N))
		for k := 0; k < 16*2*r; k++ {
			x[k] ^= v[j*16*2*r+k]
		}
		x = blockMix(x, r)
	}

	// Marshal x back into b
	for i := 0; i < 16*2*r; i++ {
		binary.LittleEndian.PutUint32(b[i*4:], x[i])
	}
}

// blockMix performs the block mix operation
func blockMix(x []uint32, r int) []uint32 {
	const blockSize = 16

	y := make([]uint32, blockSize)
	copy(y, x[(2*r-1)*blockSize:])

	result := make([]uint32, 2*r*blockSize)
	for i := 0; i < 2*r; i++ {
		t := make([]uint32, blockSize)
		for j := 0; j < blockSize; j++ {
			t[j] = x[i*blockSize+j] ^ y[j]
		}

		y = salsa20_8(t)

		for j := 0; j < blockSize; j++ {
			result[(i%2)*r*blockSize+(i/2)*blockSize+j] = y[j]
		}
	}

	return result
}

// salsa20_8 performs the Salsa20/8 core function (EXACT copy from node)
func salsa20_8(x []uint32) []uint32 {
	y := make([]uint32, len(x))
	copy(y, x)

	for i := 0; i < 4; i++ {
		// Column round
		y[12] ^= rotl(y[8]+y[4], 7)
		y[0] ^= rotl(y[12]+y[8], 9)
		y[4] ^= rotl(y[0]+y[12], 13)
		y[8] ^= rotl(y[4]+y[0], 18)

		y[13] ^= rotl(y[9]+y[5], 7)
		y[1] ^= rotl(y[13]+y[9], 9)
		y[5] ^= rotl(y[1]+y[13], 13)
		y[9] ^= rotl(y[5]+y[1], 18)

		y[14] ^= rotl(y[10]+y[6], 7)
		y[2] ^= rotl(y[14]+y[10], 9)
		y[6] ^= rotl(y[2]+y[14], 13)
		y[10] ^= rotl(y[6]+y[2], 18)

		y[15] ^= rotl(y[11]+y[7], 7)
		y[3] ^= rotl(y[15]+y[11], 9)
		y[7] ^= rotl(y[3]+y[15], 13)
		y[11] ^= rotl(y[7]+y[3], 18)

		// Row round
		y[1] ^= rotl(y[0]+y[3], 7)
		y[2] ^= rotl(y[1]+y[0], 9)
		y[3] ^= rotl(y[2]+y[1], 13)
		y[0] ^= rotl(y[3]+y[2], 18)

		y[6] ^= rotl(y[5]+y[4], 7)
		y[7] ^= rotl(y[6]+y[5], 9)
		y[4] ^= rotl(y[7]+y[6], 13)
		y[5] ^= rotl(y[4]+y[7], 18)

		y[11] ^= rotl(y[10]+y[9], 7)
		y[8] ^= rotl(y[11]+y[10], 9)
		y[9] ^= rotl(y[8]+y[11], 13)
		y[10] ^= rotl(y[9]+y[8], 18)

		y[12] ^= rotl(y[15]+y[14], 7)
		y[13] ^= rotl(y[12]+y[15], 9)
		y[14] ^= rotl(y[13]+y[12], 13)
		y[15] ^= rotl(y[14]+y[13], 18)
	}

	for i := 0; i < len(x); i++ {
		x[i] += y[i]
	}

	return x
}

// rotl performs rotate left on uint32 (EXACT copy from node)
func rotl(a, b uint32) uint32 {
	return (a << b) | (a >> (32 - b))
}
