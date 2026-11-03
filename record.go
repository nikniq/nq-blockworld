package main

import (
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Recorder captures the window into a video. F9 starts and stops it. With
// ffmpeg on the path the result is an H.264 MP4 at 30 frames a second;
// without it, a half-size 15 fps GIF capped at thirty seconds. Frames are read
// back from the GPU on the main thread and encoded on a goroutine, so a frame
// costs the readback and a copy and nothing else. There is no sound: raylib
// does not expose its mixed output.
type Recorder struct {
	Active  bool
	Path    string
	Elapsed float32
	Frames  int
	Err     string

	w, h      int // framebuffer size captured
	ow, oh    int // output size
	scale     int // framebuffer pixels per output pixel
	fps       float32
	acc       float32
	frames    chan []byte
	pool      chan []byte
	done      chan string // encoder's final error text, "" on success
	gif       bool
	finishing bool // stopped, encoder still flushing
}

const (
	recMP4FPS   = 30
	recGIFFPS   = 15
	recGIFLimit = 30 * time.Second
)

var recorder Recorder

// recordingsDir is a "recordings" folder beside the world save.
func recordingsDir() string {
	base := filepath.Dir(savePath())
	if base == "" || base == "." {
		base = "."
	}
	return filepath.Join(base, "recordings")
}

// Start begins a recording at the current render size.
func (r *Recorder) Start() {
	if r.Active {
		return
	}
	*r = Recorder{}
	r.w, r.h = rl.GetRenderWidth(), rl.GetRenderHeight()
	if r.w <= 0 || r.h <= 0 {
		r.Err = "no framebuffer"
		return
	}
	if err := os.MkdirAll(recordingsDir(), 0o755); err != nil {
		r.Err = err.Error()
		return
	}
	stamp := time.Now().Format("20060102-150405")
	r.frames = make(chan []byte, 8)
	r.pool = make(chan []byte, 8)
	r.done = make(chan string, 1)
	ff, err := exec.LookPath("ffmpeg")
	r.gif = err != nil || os.Getenv("BLOCKWORLD_GIF") != ""
	// Retina and 4K framebuffers are scaled down by whole factors: 1280 wide
	// is plenty for a clip and a quarter of the pixels to move and encode;
	// GIFs are kept to 640 so thirty seconds fits in memory.
	limit := 1280
	if r.gif {
		limit = 640
	}
	r.scale = max(1, (r.w+limit-1)/limit)
	r.ow, r.oh = (r.w/r.scale)&^1, (r.h/r.scale)&^1 // H.264 needs even sizes
	if !r.gif {
		r.Path = filepath.Join(recordingsDir(), "blockworld-"+stamp+".mp4")
		r.fps = recMP4FPS
		cmd := exec.Command(ff, "-y", "-loglevel", "error",
			"-f", "rawvideo", "-pix_fmt", "rgba", "-s", fmt.Sprintf("%dx%d", r.ow, r.oh), "-r", fmt.Sprint(recMP4FPS), "-i", "-",
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p", "-movflags", "+faststart",
			r.Path)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			r.Err = err.Error()
			return
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			r.Err = err.Error()
			return
		}
		go r.encodeMP4(cmd, stdin)
	} else {
		r.Path = filepath.Join(recordingsDir(), "blockworld-"+stamp+".gif")
		r.fps = recGIFFPS
		go r.encodeGIF()
	}
	r.Active = true
}

// Stop ends the recording. The encoder finishes in the background; Poll
// reports when the file is written.
func (r *Recorder) Stop() {
	if !r.Active {
		return
	}
	r.Active = false
	r.finishing = true
	close(r.frames)
}

// Poll returns a message once a stopped recording has been written, else "".
// With wait it blocks for the encoder (used when the game is closing).
func (r *Recorder) Poll(wait bool) string {
	if !r.finishing {
		return ""
	}
	var e string
	if wait {
		e = <-r.done
	} else {
		select {
		case e = <-r.done:
		default:
			return ""
		}
	}
	r.finishing = false
	if e != "" {
		r.Err = e
		return "Recording failed: " + e
	}
	return "Saved " + filepath.Base(r.Path)
}

// Capture is called at the top of each frame, before anything is drawn, and
// grabs the frame just presented whenever the recording's clock is due.
// Reading back before the swap missed the HUD and the held tool: raylib's 2D
// batch only reaches the framebuffer inside EndDrawing, so the finished frame
// is the one sitting in the back buffer at the start of the next.
func (r *Recorder) Capture(dt float32) {
	if !r.Active {
		return
	}
	r.Elapsed += dt
	if r.gif && time.Duration(r.Elapsed*float32(time.Second)) > recGIFLimit {
		return
	}
	r.acc += dt
	if r.acc < 1/r.fps {
		return
	}
	r.acc -= 1 / r.fps
	if r.acc > 1/r.fps {
		r.acc = 0 // a long stall: do not burst frames afterwards
	}
	if rl.GetRenderWidth() != r.w || rl.GetRenderHeight() != r.h {
		return // the window was resized mid-recording; keep the size we started with
	}
	img := rl.LoadImageFromScreen()
	if img != nil && r.Frames == 0 {
		rl.TraceLog(rl.LogInfo, "RECORD: first frame %dx%d format=%d (render %dx%d screen %dx%d)", img.Width, img.Height, img.Format, r.w, r.h, rl.GetScreenWidth(), rl.GetScreenHeight())
	}
	if img == nil || img.Data == nil || img.Format != rl.UncompressedR8g8b8a8 {
		if img != nil {
			rl.UnloadImage(img)
		}
		return
	}
	n := r.w * r.h * 4
	var buf []byte
	select {
	case buf = <-r.pool:
	default:
		buf = make([]byte, n)
	}
	copy(buf, unsafe.Slice((*byte)(img.Data), n))
	rl.UnloadImage(img)
	select {
	case r.frames <- buf:
		r.Frames++
	default:
		// The encoder is behind; dropping a frame beats stalling the game.
		r.pool <- buf
	}
}

// Secs is the recording length so far, capped for GIFs.
func (r *Recorder) Secs() float32 {
	if r.gif {
		return min(r.Elapsed, float32(recGIFLimit.Seconds()))
	}
	return r.Elapsed
}

func (r *Recorder) recycle(buf []byte) {
	select {
	case r.pool <- buf:
	default:
	}
}

// shrink samples every scale-th pixel of a captured frame into dst
// (ow x oh RGBA). Nearest sampling: the world is blocky anyway.
func (r *Recorder) shrink(buf, dst []byte) []byte {
	if r.scale == 1 && r.ow == r.w && r.oh == r.h {
		return buf
	}
	dst = dst[:r.ow*r.oh*4]
	for y := 0; y < r.oh; y++ {
		src := buf[y*r.scale*r.w*4:]
		row := dst[y*r.ow*4:]
		for x := 0; x < r.ow; x++ {
			copy(row[x*4:x*4+4], src[x*r.scale*4:x*r.scale*4+4])
		}
	}
	return dst
}

func (r *Recorder) encodeMP4(cmd *exec.Cmd, stdin io.WriteCloser) {
	var werr error
	small := make([]byte, r.ow*r.oh*4)
	for buf := range r.frames {
		if werr == nil {
			_, werr = stdin.Write(r.shrink(buf, small))
		}
		r.recycle(buf)
	}
	stdin.Close()
	err := cmd.Wait()
	switch {
	case werr != nil:
		r.done <- werr.Error()
	case err != nil:
		r.done <- "ffmpeg: " + err.Error()
	default:
		r.done <- ""
	}
}

func (r *Recorder) encodeGIF() {
	gw, gh := r.ow, r.oh
	out := &gif.GIF{}
	delay := int(100 / recGIFFPS)
	small := make([]byte, gw*gh*4)
	lut := plan9LUT()
	for buf := range r.frames {
		px := r.shrink(buf, small)
		pal := image.NewPaletted(image.Rect(0, 0, gw, gh), palette.Plan9)
		for i := 0; i < gw*gh; i++ {
			pal.Pix[i] = lut[int(px[i*4]>>3)<<10|int(px[i*4+1]>>3)<<5|int(px[i*4+2]>>3)]
		}
		r.recycle(buf)
		out.Image = append(out.Image, pal)
		out.Delay = append(out.Delay, delay)
	}
	if len(out.Image) == 0 {
		r.done <- "nothing recorded"
		return
	}
	out.Config = image.Config{ColorModel: color.Palette(palette.Plan9), Width: gw, Height: gh}
	f, err := os.Create(r.Path)
	if err != nil {
		r.done <- err.Error()
		return
	}
	if err := gif.EncodeAll(f, out); err != nil {
		f.Close()
		r.done <- err.Error()
		return
	}
	if err := f.Close(); err != nil {
		r.done <- err.Error()
		return
	}
	r.done <- ""
}

// plan9LUT maps 15-bit RGB to the nearest Plan 9 palette index. Built once
// per recording: a per-pixel nearest-colour search over 256 entries made
// a GIF frame take longer than the game's own frame.
func plan9LUT() []uint8 {
	lut := make([]uint8, 1<<15)
	for i := range lut {
		c := color.RGBA{uint8(i>>10&31) << 3, uint8(i>>5&31) << 3, uint8(i&31) << 3, 255}
		lut[i] = uint8(color.Palette(palette.Plan9).Index(c))
	}
	return lut
}
