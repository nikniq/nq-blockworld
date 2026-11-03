package main

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Every piece renders in reasonable time, is neither silent nor clipped, and
// has the length its tempo and bar count imply. BLOCKWORLD_MUSIC_DUMP=dir
// also writes them out as WAV files to listen to.
func TestMusicPieces(t *testing.T) {
	dump := os.Getenv("BLOCKWORLD_MUSIC_DUMP")
	for mood := Mood(0); mood < numMoods; mood++ {
		start := time.Now()
		data, frames := renderPiece(mood)
		took := time.Since(start)
		s := specFor(mood)
		want := float32(s.bars) * 4 * 60 / s.bpm
		if got := float32(frames) / sampleRate; got < want || got > want+3 {
			t.Errorf("%s: length %.1fs, want about %.1fs", moodNames[mood], got, want)
		}
		var sum float64
		peak, clipped := 0.0, 0
		for i := 0; i < frames*2; i++ {
			v := float64(int16(binary.LittleEndian.Uint16(data[i*2:]))) / 32768
			sum += v * v
			peak = math.Max(peak, math.Abs(v))
			if math.Abs(v) > 0.995 {
				clipped++
			}
		}
		rms := math.Sqrt(sum / float64(frames*2))
		if rms < 0.05 || rms > 0.25 || peak < 0.3 || clipped > 0 {
			t.Errorf("%s: rms %.3f peak %.3f clipped %d samples", moodNames[mood], rms, peak, clipped)
		}
		if took > 8*time.Second {
			t.Errorf("%s: rendering took %v", moodNames[mood], took)
		}
		t.Logf("%s: %.1fs rms %.3f peak %.3f rendered in %v", moodNames[mood], float32(frames)/sampleRate, rms, peak, took)
		if dump != "" {
			writeWav(filepath.Join(dump, "music-"+moodNames[mood]+".wav"), data, frames)
		}
	}
}

func writeWav(path string, pcm []byte, frames int) {
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(pcm)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], sampleRate)
	binary.LittleEndian.PutUint32(h[28:], sampleRate*4)
	binary.LittleEndian.PutUint16(h[32:], 4)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(pcm)))
	os.WriteFile(path, append(h, pcm...), 0o644)
}
