package notification

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// SoundSink plays notification sounds server-side via system audio.
// Uses paplay (PulseAudio/PipeWire) on Linux, afplay on macOS.
type SoundSink struct {
	mu          sync.Mutex
	enabled     bool
	audioDevice string
	wavDir      string // temp dir with generated WAV files
}

func NewSoundSink(enabled bool, audioDevice string) *SoundSink {
	s := &SoundSink{enabled: enabled, audioDevice: audioDevice}
	// Use a fixed path so sounds persist and don't leak temp dirs
	home, _ := os.UserHomeDir()
	s.wavDir = filepath.Join(home, ".websessions", "sounds")
	_ = os.MkdirAll(s.wavDir, 0755)
	s.generateWAVs()
	return s
}

func (s *SoundSink) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
}

func (s *SoundSink) SetAudioDevice(device string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audioDevice = device
}

func (s *SoundSink) Send(event SessionEvent) error {
	s.mu.Lock()
	enabled := s.enabled
	device := s.audioDevice
	s.mu.Unlock()

	if !enabled {
		return nil
	}

	var wavFile string
	switch event.Type {
	case EventCompleted:
		wavFile = "completed.wav"
	case EventErrored:
		wavFile = "errored.wav"
	case EventWaiting:
		wavFile = "waiting.wav"
	default:
		return nil
	}

	path := filepath.Join(s.wavDir, wavFile)
	if _, err := os.Stat(path); err != nil {
		return nil
	}

	go s.play(path, device)
	return nil
}

func (s *SoundSink) play(path, device string) {
	switch runtime.GOOS {
	case "linux":
		args := []string{}
		if device != "" {
			args = append(args, "--device="+device)
		}
		args = append(args, path)
		// Try paplay first (PulseAudio/PipeWire), fall back to aplay (ALSA)
		if err := exec.Command("paplay", args...).Run(); err != nil {
			_ = exec.Command("aplay", path).Run()
		}
	case "darwin":
		_ = exec.Command("afplay", path).Run()
	}
}


// ListAudioDevices returns available audio output devices.
func ListAudioDevices() []AudioDevice {
	switch runtime.GOOS {
	case "linux":
		return listPulseDevices()
	default:
		return nil
	}
}

type AudioDevice struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func listPulseDevices() []AudioDevice {
	out, err := exec.Command("pactl", "list", "short", "sinks").Output()
	if err != nil {
		return nil
	}
	var devices []AudioDevice
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			name := fields[1]
			// Get description
			desc := name
			descOut, err := exec.Command("pactl", "list", "sinks").Output()
			if err == nil {
				lines := strings.Split(string(descOut), "\n")
				for i, l := range lines {
					if strings.Contains(l, "Name: "+name) {
						for j := i + 1; j < len(lines); j++ {
							if strings.Contains(lines[j], "Description:") {
								desc = strings.TrimSpace(strings.SplitN(lines[j], ":", 2)[1])
								break
							}
						}
						break
					}
				}
			}
			devices = append(devices, AudioDevice{Name: name, Description: desc})
		}
	}
	return devices
}

// generateWAVs creates small WAV tone files for each notification type.
func (s *SoundSink) generateWAVs() {
	// Completed: ascending two-note chime (C5 → G5)
	s.writeWAV("completed.wav", generateTone([]float64{523, 784}, []float64{0.15, 0.25}, 0.3))
	// Errored: descending two-note (G4 → C4)
	s.writeWAV("errored.wav", generateTone([]float64{392, 262}, []float64{0.2, 0.3}, 0.3))
	// Waiting: single gentle ping (E5)
	s.writeWAV("waiting.wav", generateTone([]float64{659}, []float64{0.3}, 0.2))
}

func (s *SoundSink) writeWAV(name string, samples []byte) {
	path := filepath.Join(s.wavDir, name)
	sampleRate := 44100
	numSamples := len(samples) / 2 // 16-bit samples
	dataSize := numSamples * 2
	fileSize := 36 + dataSize

	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck

	// WAV header
	_, _ = f.Write([]byte("RIFF"))
	writeLE32(f, uint32(fileSize))
	_, _ = f.Write([]byte("WAVE"))
	_, _ = f.Write([]byte("fmt "))
	writeLE32(f, 16)           // chunk size
	writeLE16(f, 1)            // PCM
	writeLE16(f, 1)            // mono
	writeLE32(f, uint32(sampleRate))
	writeLE32(f, uint32(sampleRate*2)) // byte rate
	writeLE16(f, 2)            // block align
	writeLE16(f, 16)           // bits per sample
	_, _ = f.Write([]byte("data"))
	writeLE32(f, uint32(dataSize))
	_, _ = f.Write(samples)
}

func generateTone(freqs []float64, durations []float64, volume float64) []byte {
	sampleRate := 44100.0
	var samples []byte
	offset := 0.0
	for i, freq := range freqs {
		dur := durations[i]
		numSamples := int(dur * sampleRate)
		for j := 0; j < numSamples; j++ {
			t := float64(j) / sampleRate
			// Sine wave with fade in/out envelope
			envelope := 1.0
			fadeLen := 0.02
			if t < fadeLen {
				envelope = t / fadeLen
			} else if t > dur-fadeLen {
				envelope = (dur - t) / fadeLen
			}
			sample := math.Sin(2*math.Pi*freq*(t+offset)) * volume * envelope
			// 16-bit signed PCM
			val := int16(sample * 32767)
			samples = append(samples, byte(val), byte(val>>8))
		}
		offset += durations[i]
	}
	return samples
}

func writeLE16(f *os.File, v uint16) {
	_, _ = f.Write([]byte{byte(v), byte(v >> 8)})
}

func writeLE32(f *os.File, v uint32) {
	_, _ = f.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
}
