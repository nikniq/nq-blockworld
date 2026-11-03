package main

import (
	"encoding/binary"
	"math"
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Procedural music. Five pieces are composed and rendered at start-up, each
// for a mood of play, and the game crossfades between them as the mood
// changes: a bright theme by day, a slow minor one at night, drones and
// bells in the caves, a driving piece when hostiles close in, and a calm
// theme on the menu. Every piece is built from a chord progression in a
// mode, with a generated melody in call-and-response phrases, a filtered
// pad, a bass line, an arpeggio and light percussion, mixed in stereo
// through a small reverb. Rendering happens on background goroutines so
// the game starts at once; a piece joins in when it is ready.

type Mood int

const (
	MoodNone Mood = iota - 1
	MoodMenu
	MoodDay
	MoodNight
	MoodCave
	MoodDanger
	numMoods
)

var moodNames = [numMoods]string{"Menu", "Day", "Night", "Cave", "Danger"}

// MusicBox owns the rendered pieces and their fades.
type MusicBox struct {
	tracks  [numMoods]musicTrack
	ready   chan renderedPiece
	Current Mood
	hold    float32 // seconds danger music lingers after the last threat
}

type musicTrack struct {
	sound  rl.Sound
	loaded bool
	vol    float32 // current fade level 0..1
	target float32
	length float32 // seconds
	played float32 // seconds since it was last started
}

type renderedPiece struct {
	mood   Mood
	data   []byte
	frames int
}

// NewMusicBox starts rendering every piece in the background.
func NewMusicBox() *MusicBox {
	m := &MusicBox{ready: make(chan renderedPiece, numMoods), Current: MoodNone}
	for mood := Mood(0); mood < numMoods; mood++ {
		go func(mood Mood) {
			data, frames := renderPiece(mood)
			m.ready <- renderedPiece{mood, data, frames}
		}(mood)
	}
	return m
}

// Update fades toward the wanted mood. Call it once a frame on the main thread.
func (m *MusicBox) Update(want Mood, dt float32, vol float32) {
	// Pick up pieces that finished rendering.
	for {
		select {
		case p := <-m.ready:
			w := rl.NewWave(uint32(p.frames), sampleRate, 16, 2, p.data)
			t := &m.tracks[p.mood]
			t.sound = rl.LoadSoundFromWave(w)
			t.loaded = true
			t.length = float32(p.frames) / sampleRate
		default:
			goto loaded
		}
	}
loaded:
	if want != m.Current {
		m.Current = want
	}
	for i := range m.tracks {
		t := &m.tracks[i]
		if !t.loaded {
			continue
		}
		t.target = 0
		if Mood(i) == want {
			t.target = 1
		}
		// Danger cuts in fast; everything else drifts over a few seconds.
		rate := dt / 3
		if Mood(i) == MoodDanger || want == MoodDanger {
			rate = dt / 1.2
		}
		if t.vol < t.target {
			t.vol = min(t.target, t.vol+rate)
		} else if t.vol > t.target {
			t.vol = max(t.target, t.vol-rate)
		}
		playing := rl.IsSoundPlaying(t.sound)
		switch {
		case t.vol <= 0 && playing:
			rl.StopSound(t.sound)
		case t.vol > 0 && !playing && t.target > 0:
			// Start, or loop round: the pieces end on a held tonic so the join reads as a phrase.
			rl.SetSoundVolume(t.sound, t.vol*vol)
			rl.PlaySound(t.sound)
			t.played = 0
		case playing:
			t.played += dt
			rl.SetSoundVolume(t.sound, t.vol*vol)
		}
	}
}

// Stop silences everything at once.
func (m *MusicBox) Stop() {
	for i := range m.tracks {
		t := &m.tracks[i]
		t.vol, t.target = 0, 0
		if t.loaded && rl.IsSoundPlaying(t.sound) {
			rl.StopSound(t.sound)
		}
	}
	m.Current = MoodNone
}

// Close unloads the pieces.
func (m *MusicBox) Close() {
	for i := range m.tracks {
		if m.tracks[i].loaded {
			rl.UnloadSound(m.tracks[i].sound)
		}
	}
}

// ---------- composition ----------

var (
	scaleMajor    = []int{0, 2, 4, 5, 7, 9, 11}
	scaleMinor    = []int{0, 2, 3, 5, 7, 8, 10}
	scaleDorian   = []int{0, 2, 3, 5, 7, 9, 10}
	scalePhrygian = []int{0, 1, 3, 5, 7, 8, 10}
	scaleLydian   = []int{0, 2, 4, 6, 7, 9, 11}
)

type leadKind int

const (
	leadPluck leadKind = iota
	leadFlute
	leadBell
	leadSquare
)

// pieceSpec is everything that distinguishes one mood's piece.
type pieceSpec struct {
	bpm     float32
	root    int   // midi note of the key
	scale   []int // the mode
	prog    []int // chord root degrees, one per bar, looped
	bars    int
	lead    leadKind
	leadVol float32
	leadLow int // lowest lead note as a scale degree above the root
	arp     bool
	drums   int // 0 none, 1 light shaker, 2 full kit, 3 slow toms
	bass    int // 0 drone, 1 root and fifth, 2 walking, 3 driving eighths
	rhythms [][]float32
	rest    float32 // chance a melody note is a rest
	padCut  float32 // pad low-pass cutoff in Hz
	padVol  float32
	seed    int64
}

func specFor(mood Mood) pieceSpec {
	switch mood {
	case MoodDay:
		return pieceSpec{bpm: 96, root: 60, scale: scaleMajor, prog: []int{0, 4, 5, 3, 0, 4, 1, 4}, bars: 32,
			lead: leadPluck, leadVol: 0.62, leadLow: 7, arp: true, drums: 1, bass: 2,
			rhythms: [][]float32{{1, 1, 2}, {0.5, 0.5, 1, 2}, {2, 1, 1}, {1, 0.5, 0.5, 1, 1}, {1.5, 0.5, 2}, {3, 1}, {2, 2}},
			rest:    0.12, padCut: 1200, padVol: 0.09, seed: 7}
	case MoodNight:
		return pieceSpec{bpm: 62, root: 57, scale: scaleMinor, prog: []int{0, 5, 2, 6, 0, 5, 3, 4}, bars: 32,
			lead: leadFlute, leadVol: 0.5, leadLow: 7, arp: false, drums: 3, bass: 1,
			rhythms: [][]float32{{2, 2}, {4}, {3, 1}, {1, 3}, {2, 1, 1}},
			rest:    0.25, padCut: 700, padVol: 0.13, seed: 11}
	case MoodCave:
		return pieceSpec{bpm: 54, root: 50, scale: scaleDorian, prog: []int{0, 0, 3, 0, 0, 6, 3, 0}, bars: 32,
			lead: leadBell, leadVol: 0.5, leadLow: 14, arp: false, drums: 0, bass: 0,
			rhythms: [][]float32{{4}, {2, 2}, {3, 1}, {1, 3}},
			rest:    0.45, padCut: 420, padVol: 0.16, seed: 13}
	case MoodDanger:
		return pieceSpec{bpm: 136, root: 52, scale: scalePhrygian, prog: []int{0, 0, 1, 0, 0, 0, 6, 1}, bars: 32,
			lead: leadSquare, leadVol: 0.4, leadLow: 7, arp: true, drums: 2, bass: 3,
			rhythms: [][]float32{{0.5, 0.5, 0.5, 0.5, 1, 1}, {1, 0.5, 0.5, 1, 1}, {0.5, 0.5, 1, 0.5, 0.5, 1}, {2, 1, 1}, {0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 1}},
			rest:    0.1, padCut: 900, padVol: 0.08, seed: 17}
	default: // menu
		return pieceSpec{bpm: 74, root: 65, scale: scaleLydian, prog: []int{0, 1, 0, 1, 5, 1, 0, 0}, bars: 32,
			lead: leadPluck, leadVol: 0.42, leadLow: 7, arp: true, drums: 0, bass: 1,
			rhythms: [][]float32{{2, 2}, {1, 1, 2}, {3, 1}, {1.5, 0.5, 2}, {4}},
			rest:    0.2, padCut: 900, padVol: 0.12, seed: 19}
	}
}

// degree converts a scale degree (any integer) to a midi note.
func (s *pieceSpec) degree(d int) int {
	oct, i := d/7, d%7
	if i < 0 {
		i += 7
		oct--
	}
	return s.root + 12*oct + s.scale[i]
}

func midiHz(m int) float32 { return 440 * float32(math.Pow(2, float64(m-69)/12)) }

type note struct {
	start, dur float32 // in beats
	deg        int     // scale degree
	vel        float32
}

// melody writes the lead line: four 8-bar phrases in call-and-response,
// each bar's rhythm drawn from the mood's patterns, pitches walking through
// the scale and landing on chord tones on the strong beats.
func (s *pieceSpec) melody(rng *rand.Rand) []note {
	var out []note
	phrase := func(bar0 int, bars int, ending bool) []note {
		var ph []note
		cur := s.leadLow + 7 // start an octave above the lowest allowed
		for b := 0; b < bars; b++ {
			bar := bar0 + b
			chordRoot := s.prog[bar%len(s.prog)]
			pattern := s.rhythms[rng.Intn(len(s.rhythms))]
			last := ending && b == bars-1
			if last {
				pattern = []float32{4}
			}
			pos := float32(0)
			for _, d := range pattern {
				strong := pos == 0 || pos == 2
				if !last && rng.Float32() < s.rest && !strong {
					pos += d
					continue
				}
				var deg int
				switch {
				case last:
					deg = nearestChordTone(cur, chordRoot, 0) // resolve to the root
				case strong || rng.Float32() < 0.55:
					deg = nearestChordTone(cur+rng.Intn(3)-1, chordRoot, rng.Intn(3))
				default:
					step := rng.Intn(5) - 2
					if step == 0 {
						step = 1
					}
					deg = cur + step
				}
				// Keep inside a comfortable range; leap back toward the middle when out.
				for deg < s.leadLow {
					deg += 7
				}
				for deg > s.leadLow+13 {
					deg -= 7
				}
				vel := 0.8 + rng.Float32()*0.2
				if strong {
					vel += 0.1
				}
				ph = append(ph, note{float32(bar)*4 + pos, d, deg, vel})
				cur = deg
				pos += d
			}
		}
		return ph
	}
	a := phrase(0, 8, false)
	out = append(out, a...)
	// Second phrase answers: the first half of A again, then a new half.
	for _, n := range a {
		if n.start < 16 {
			n.start += 32
			out = append(out, n)
		}
	}
	out = append(out, phrase(12, 4, false)...)
	out = append(out, phrase(16, 8, false)...)
	for _, n := range a {
		if n.start < 16 {
			n.start += 96
			out = append(out, n)
		}
	}
	out = append(out, phrase(28, 4, true)...)
	return out
}

// nearestChordTone snaps a degree to a tone of the triad on chordRoot: the
// root, third or fifth (which = 0, 1, 2), in the octave nearest to deg.
func nearestChordTone(deg, chordRoot, which int) int {
	tone := chordRoot + 2*which
	best := tone
	for o := -3; o <= 4; o++ {
		if c := tone + 7*o; abs(c-deg) < abs(best-deg) {
			best = c
		}
	}
	return best
}

// ---------- rendering ----------

// mixer accumulates stereo float samples.
type mixer struct {
	L, R []float32
}

func newMixer(frames int) *mixer { return &mixer{make([]float32, frames), make([]float32, frames)} }

// add renders gen (t in seconds from the note's start) into the buffers at
// a start time, panned -1..1.
func (m *mixer) add(start, dur, pan float32, gen func(t float32) float32) {
	i0 := int(start * sampleRate)
	n := int(dur * sampleRate)
	gl := float32(math.Sqrt(float64((1 - pan) / 2)))
	gr := float32(math.Sqrt(float64((1 + pan) / 2)))
	for i := 0; i < n; i++ {
		j := i0 + i
		if j < 0 || j >= len(m.L) {
			continue
		}
		v := gen(float32(i) / sampleRate)
		m.L[j] += v * gl
		m.R[j] += v * gr
	}
}

func sin32(x float32) float32 { return float32(math.Sin(float64(x))) }
func expf(x float32) float32  { return float32(math.Exp(float64(x))) }
func trif(phase float32) float32 {
	p := phase - float32(math.Floor(float64(phase)))
	return 1 - 4*float32(math.Abs(float64(p-0.5)))
}

const twoPi = 2 * math.Pi

// Instruments return a generator for one note.

func instPluck(f, vel, dur float32) func(float32) float32 {
	return func(t float32) float32 {
		env := expf(-2.6*t) * min(1, t*120)
		return vel * env * (sin32(twoPi*f*t) + 0.45*sin32(twoPi*2*f*t)*expf(-3*t) + 0.2*sin32(twoPi*3*f*t)*expf(-5*t) + 0.12*sin32(twoPi*4*f*t)*expf(-7*t))
	}
}

// instFlute carries its phase from sample to sample so the vibrato bends the
// pitch gently instead of running away on long notes.
func instFlute(f, vel, dur float32) func(float32) float32 {
	var phase float32
	return func(t float32) float32 {
		att := min(1, t/0.18)
		rel := min(1, max(0, (dur-t)/0.25))
		vib := 1 + 0.004*sin32(twoPi*5*t)*min(1, t/0.6)
		phase += f * vib / sampleRate
		x := twoPi * phase
		return vel * att * rel * 0.9 * (sin32(x) + 0.18*sin32(2*x) + 0.06*sin32(3*x))
	}
}

func instBell(f, vel, dur float32) func(float32) float32 {
	return func(t float32) float32 {
		return vel * 0.7 * (sin32(twoPi*f*t)*expf(-1.1*t) + 0.5*sin32(twoPi*2.76*f*t)*expf(-2.5*t) + 0.25*sin32(twoPi*5.4*f*t)*expf(-4*t) + 0.3*sin32(twoPi*0.5*f*t)*expf(-0.8*t))
	}
}

func instSquare(f, vel, dur float32) func(float32) float32 {
	return func(t float32) float32 {
		att := min(1, t/0.006)
		rel := min(1, max(0, (dur-t)/0.05))
		x := twoPi * f * t
		y := twoPi * f * 1.004 * t
		return vel * att * rel * 0.5 * (sin32(x) + 0.33*sin32(3*x) + 0.2*sin32(5*x) + 0.8*sin32(y) + 0.25*sin32(3*y))
	}
}

func instBass(f, vel, dur float32) func(float32) float32 {
	return func(t float32) float32 {
		att := min(1, t/0.012)
		env := 0.45 + 0.55*expf(-3*t)
		rel := min(1, max(0, (dur-t)/0.06))
		return vel * att * env * rel * (sin32(twoPi*f*t) + 0.35*sin32(twoPi*2*f*t) + 0.2*trif(f*t))
	}
}

func instKick(vel float32) func(float32) float32 {
	var phase float32
	return func(t float32) float32 {
		phase += (48 + 90*expf(-28*t)) / sampleRate
		return vel * (sin32(twoPi*phase)*expf(-8*t) + (rand.Float32()*2-1)*0.3*expf(-90*t))
	}
}

func instSnare(vel float32) func(float32) float32 {
	return func(t float32) float32 {
		return vel * ((rand.Float32()*2-1)*0.7*expf(-16*t) + sin32(twoPi*185*t)*0.5*expf(-24*t))
	}
}

func instHat(vel float32, open bool) func(float32) float32 {
	var prev float32
	decay := float32(70)
	if open {
		decay = 18
	}
	return func(t float32) float32 {
		n := rand.Float32()*2 - 1
		hp := n - prev // crude high-pass
		prev = n
		return vel * hp * 0.5 * expf(-decay*t)
	}
}

func instShaker(vel float32) func(float32) float32 {
	var p1, p2 float32
	return func(t float32) float32 {
		n := rand.Float32()*2 - 1
		hp := n - 2*p1 + p2 // second-order high-pass: just the sizzle
		p2, p1 = p1, n
		return vel * hp * 0.07 * sin32(math.Pi*min(1, t/0.09))
	}
}

func instTom(vel float32) func(float32) float32 {
	var phase float32
	return func(t float32) float32 {
		phase += (70 + 30*expf(-10*t)) / sampleRate
		return vel * sin32(twoPi*phase) * expf(-5*t)
	}
}

// pad renders a chord as detuned triangles with a slow swell. It is drawn
// into its own bus so the whole thing can be low-passed afterwards.
func padGen(freqs []float32, dur, vol float32, side float32) func(float32) float32 {
	return func(t float32) float32 {
		att := min(1, t/1.2)
		rel := min(1, max(0, (dur-t)/1.2))
		v := float32(0)
		for i, f := range freqs {
			det := 1 + side*0.0025*float32(i+1)
			v += trif(f*det*t) + 0.6*sin32(twoPi*f*t)
		}
		return vol * att * rel * v / float32(len(freqs))
	}
}

// lowpass runs a one-pole filter over a buffer in place, with a slow sweep
// of the cutoff so the pad breathes.
func lowpass(buf []float32, cutoff float32, sweep float32) {
	var y float32
	for i := range buf {
		c := cutoff * (1 + 0.35*sin32(twoPi*0.05*float32(i)/sampleRate+sweep))
		a := 1 - expf(-twoPi*c/sampleRate)
		y += a * (buf[i] - y)
		buf[i] = y
	}
}

// reverb adds a small stereo room: two feedback combs per side with damping,
// cross-fed a little so the space feels shared.
func reverb(L, R []float32, send float32) {
	sr := float32(sampleRate)
	type comb struct {
		buf  []float32
		i    int
		damp float32
	}
	mk := func(sec float32) *comb { return &comb{buf: make([]float32, int(sec*sampleRate))} }
	cl := []*comb{mk(0.0297), mk(0.0371), mk(0.0411), mk(0.0437)}
	cr := []*comb{mk(0.0313), mk(0.0353), mk(0.0427), mk(0.0449)}
	run := func(cs []*comb, in float32) float32 {
		var out float32
		for _, c := range cs {
			v := c.buf[c.i]
			c.damp += 0.3 * (v - c.damp)
			c.buf[c.i] = in + c.damp*0.78
			c.i++
			if c.i == len(c.buf) {
				c.i = 0
			}
			out += v
		}
		return out / float32(len(cs))
	}
	// A longer pre-delay echo gives the sense of distance.
	echoL, echoR := make([]float32, int(0.23*sr)), make([]float32, int(0.29*sr))
	var ei, ej int
	for i := range L {
		inL := L[i]*0.9 + R[i]*0.1
		inR := R[i]*0.9 + L[i]*0.1
		wl := run(cl, inL)
		wr := run(cr, inR)
		el, er := echoL[ei], echoR[ej]
		echoL[ei] = inL + er*0.35
		echoR[ej] = inR + el*0.35
		ei++
		if ei == len(echoL) {
			ei = 0
		}
		ej++
		if ej == len(echoR) {
			ej = 0
		}
		L[i] += send * (wl + 0.5*el)
		R[i] += send * (wr + 0.5*er)
	}
}

// renderPiece composes and renders one mood's piece to interleaved 16-bit stereo.
func renderPiece(mood Mood) ([]byte, int) {
	s := specFor(mood)
	rng := rand.New(rand.NewSource(s.seed))
	beat := 60 / s.bpm
	total := float32(s.bars) * 4 * beat
	frames := int((total + 2.5) * sampleRate) // tail for the reverb and the last note
	m := newMixer(frames)
	pad := newMixer(frames)
	chordFreqs := func(bar int, low bool) []float32 {
		r := s.prog[bar%len(s.prog)]
		degs := []int{r, r + 2, r + 4, r + 6}
		if low {
			degs = []int{r - 7, r, r + 2, r + 4}
		}
		out := make([]float32, len(degs))
		for i, d := range degs {
			out[i] = midiHz(s.degree(d))
		}
		return out
	}
	// Pad: one chord per bar, held across bars when the chord repeats.
	for bar := 0; bar < s.bars; {
		end := bar + 1
		for end < s.bars && s.prog[end%len(s.prog)] == s.prog[bar%len(s.prog)] {
			end++
		}
		dur := float32(end-bar) * 4 * beat
		start := float32(bar) * 4 * beat
		pad.add(start, dur+0.8, -0.6, padGen(chordFreqs(bar, mood == MoodCave), dur+0.8, s.padVol, 1))
		pad.add(start, dur+0.8, 0.6, padGen(chordFreqs(bar, mood == MoodCave), dur+0.8, s.padVol, -1))
		bar = end
	}
	lowpass(pad.L, s.padCut, 0)
	lowpass(pad.R, s.padCut, 1.3)
	for i := range m.L {
		m.L[i] += pad.L[i]
		m.R[i] += pad.R[i]
	}
	// Bass.
	for bar := 0; bar < s.bars; bar++ {
		root := s.prog[bar%len(s.prog)]
		t0 := float32(bar) * 4 * beat
		play := func(atBeat, durBeats float32, deg int, vel float32) {
			f := midiHz(s.degree(deg) - 24)
			m.add(t0+atBeat*beat, durBeats*beat, 0, instBass(f, vel*0.6, durBeats*beat))
		}
		switch s.bass {
		case 0:
			play(0, 4, root, 0.5)
		case 1:
			play(0, 2, root, 0.55)
			play(2, 2, root+4, 0.45)
		case 2:
			play(0, 1, root, 0.55)
			play(1, 1, root+4, 0.4)
			play(2, 1, root, 0.5)
			if bar%2 == 1 {
				play(3, 1, root+2, 0.4)
			} else {
				play(3, 1, root+4, 0.4)
			}
		case 3:
			for e := 0; e < 8; e++ {
				deg := root
				if e == 3 || e == 7 {
					deg = root - 1
				}
				if e == 5 {
					deg = root + 7
				}
				play(float32(e)*0.5, 0.45, deg, 0.5)
			}
		}
	}
	// Arpeggio: sixteenths through the chord, quiet and off to one side.
	if s.arp {
		step := beat / 4
		if mood == MoodDanger {
			step = beat / 2
		}
		for bar := 0; bar < s.bars; bar++ {
			root := s.prog[bar%len(s.prog)]
			tones := []int{root, root + 2, root + 4, root + 7, root + 4, root + 2}
			t0 := float32(bar) * 4 * beat
			n := int(4 * beat / step)
			for k := 0; k < n; k++ {
				deg := tones[k%len(tones)] + 7
				f := midiHz(s.degree(deg))
				vel := float32(0.16)
				if k%4 == 0 {
					vel = 0.22
				}
				if mood == MoodDanger {
					m.add(t0+float32(k)*step, step*0.9, -0.45, instSquare(f, vel*0.8, step*0.9))
				} else {
					m.add(t0+float32(k)*step, step*2.5, -0.45, instPluck(f, vel, step*2.5))
				}
			}
		}
	}
	// Lead.
	for _, n := range s.melody(rng) {
		f := midiHz(s.degree(n.deg))
		start := n.start * beat
		dur := n.dur * beat
		vel := n.vel * s.leadVol
		switch s.lead {
		case leadPluck:
			m.add(start, dur+1.5, 0.3, instPluck(f, vel, dur))
		case leadFlute:
			m.add(start, dur, 0.25, instFlute(f, vel, dur))
		case leadBell:
			m.add(start, dur+3, 0.3, instBell(f, vel, dur))
		case leadSquare:
			m.add(start, dur*0.9, 0.3, instSquare(f, vel, dur*0.9))
		}
	}
	// Percussion.
	for bar := 0; bar < s.bars; bar++ {
		t0 := float32(bar) * 4 * beat
		switch s.drums {
		case 1:
			for e := 0; e < 8; e++ {
				vel := float32(0.08)
				if e%2 == 1 {
					vel = 0.14
				}
				m.add(t0+float32(e)*beat/2, 0.1, 0.5, instShaker(vel))
			}
			if bar%4 == 3 {
				m.add(t0+3.5*beat, 0.5, -0.2, instTom(0.25))
			}
		case 2:
			for e := 0; e < 8; e++ {
				t := t0 + float32(e)*beat/2
				m.add(t, 0.12, 0.4, instHat(0.16, false))
				if e == 0 || e == 4 || (e == 6 && bar%2 == 1) || (e == 3 && bar%4 == 2) {
					m.add(t, 0.3, 0, instKick(0.7))
				}
				if e == 2 || e == 6 {
					m.add(t, 0.25, -0.1, instSnare(0.38))
				}
			}
			if bar%8 == 7 {
				for e := 0; e < 4; e++ {
					m.add(t0+2*beat+float32(e)*beat/2, 0.25, 0.1, instSnare(0.4+0.1*float32(e)))
				}
				m.add(t0+3.5*beat, 0.4, 0.3, instHat(0.2, true))
			}
		case 3:
			if bar%2 == 0 {
				m.add(t0, 0.8, -0.3, instTom(0.35))
			}
			if bar%4 == 2 {
				m.add(t0+2*beat, 0.8, 0.3, instTom(0.25))
			}
		}
	}
	send := float32(0.28)
	if mood == MoodCave {
		send = 0.5
	}
	if mood == MoodDanger {
		send = 0.18
	}
	reverb(m.L, m.R, send)
	// Level to a target loudness, then soft-clip only the rare peaks so the
	// dynamics of the playing survive.
	var sum float64
	for i := range m.L {
		sum += float64(m.L[i]*m.L[i] + m.R[i]*m.R[i])
	}
	rms := float32(math.Sqrt(sum / float64(2*len(m.L))))
	pre := 0.11 / max(rms, 0.001)
	peak := float32(0.001)
	for i := range m.L {
		m.L[i] = float32(math.Tanh(float64(m.L[i] * pre)))
		m.R[i] = float32(math.Tanh(float64(m.R[i] * pre)))
		peak = max(peak, max(float32(math.Abs(float64(m.L[i]))), float32(math.Abs(float64(m.R[i])))))
	}
	gain := min(float32(1), 0.9/peak)
	// Fade the tail so the loop restarts cleanly.
	data := make([]byte, frames*4)
	for i := 0; i < frames; i++ {
		fade := float32(1)
		if left := frames - i; left < int(0.4*sampleRate) {
			fade = float32(left) / (0.4 * sampleRate)
		}
		l := clamp(m.L[i]*gain*fade, -1, 1)
		r := clamp(m.R[i]*gain*fade, -1, 1)
		binary.LittleEndian.PutUint16(data[i*4:], uint16(int16(l*32000)))
		binary.LittleEndian.PutUint16(data[i*4+2:], uint16(int16(r*32000)))
	}
	return data, frames
}
