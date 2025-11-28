package main

import (
	"strings"
	"testing"
	"time"
)

// BenchmarkHashing benchmarks the hashing operation
func BenchmarkHashing(b *testing.B) {
	// Create a sample route chunk (typical size ~500 bytes)
	sampleRoute := `Destination: 1.0.0.0/24          
     Protocol: IBGP               Process ID: 0              
   Preference: 255                      Cost: 0              
      NextHop: 172.31.251.131      Neighbour: 172.31.251.131
        State: Active Adv Relied         Age: 27d02h01m21s        
          Tag: 0                    Priority: low            
        Label: NULL                  QoSInfo: 0x0           
   IndirectID: 0x6005CE7            Instance:                                 
 RelayNextHop: 172.31.254.50       Interface: Global-VE1.75
     TunnelID: 0x0                     Flags: RD`

	data := []byte(sampleRoute)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashChunk(data)
	}
}

// BenchmarkParsing simulates parsing a route (string operations)
func BenchmarkParsing(b *testing.B) {
	sampleRoute := `Destination: 1.0.0.0/24          
     Protocol: IBGP               Process ID: 0              
   Preference: 255                      Cost: 0              
      NextHop: 172.31.251.131      Neighbour: 172.31.251.131
        State: Active Adv Relied         Age: 27d02h01m21s        
          Tag: 0                    Priority: low            
        Label: NULL                  QoSInfo: 0x0           
   IndirectID: 0x6005CE7            Instance:                                 
 RelayNextHop: 172.31.254.50       Interface: Global-VE1.75
     TunnelID: 0x0                     Flags: RD`

	lines := []string{sampleRoute}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate parsing operations
		for _, line := range lines {
			if len(line) > 0 {
				parts := strings.Fields(line)
				_ = parts
				_ = strings.Contains(line, "Destination:")
				_ = strings.Contains(line, "Protocol:")
				_ = strings.Contains(line, "NextHop:")
			}
		}
	}
}

// TestHashingVsParsing compares hashing vs parsing performance
func TestHashingVsParsing(t *testing.T) {
	sampleRoute := `Destination: 1.0.0.0/24          
     Protocol: IBGP               Process ID: 0              
   Preference: 255                      Cost: 0              
      NextHop: 172.31.251.131      Neighbour: 172.31.251.131
        State: Active Adv Relied         Age: 27d02h01m21s        
          Tag: 0                    Priority: low            
        Label: NULL                  QoSInfo: 0x0           
   IndirectID: 0x6005CE7            Instance:                                 
 RelayNextHop: 172.31.254.50       Interface: Global-VE1.75
     TunnelID: 0x0                     Flags: RD`

	data := []byte(sampleRoute)
	iterations := 10000

	// Benchmark hashing
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_ = hashChunk(data)
	}
	hashDuration := time.Since(start)

	// Benchmark parsing
	start = time.Now()
	lines := []string{sampleRoute}
	for i := 0; i < iterations; i++ {
		for _, line := range lines {
			if len(line) > 0 {
				parts := strings.Fields(line)
				_ = parts
				_ = strings.Contains(line, "Destination:")
				_ = strings.Contains(line, "Protocol:")
				_ = strings.Contains(line, "NextHop:")
			}
		}
	}
	parseDuration := time.Since(start)

	t.Logf("Hashing %d iterations: %v (%.2f ops/sec)", iterations, hashDuration, float64(iterations)/hashDuration.Seconds())
	t.Logf("Parsing %d iterations: %v (%.2f ops/sec)", iterations, parseDuration, float64(iterations)/parseDuration.Seconds())
	t.Logf("Hashing is %.2fx faster", parseDuration.Seconds()/hashDuration.Seconds())
}

