// whisp: hold a key, speak, release — the transcript is typed into the focused window.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tamasmagyar/whisp/internal/audio"
	"github.com/tamasmagyar/whisp/internal/config"
	"github.com/tamasmagyar/whisp/internal/hotkey"
	"github.com/tamasmagyar/whisp/internal/inject"
	"github.com/tamasmagyar/whisp/internal/meeting"
	"github.com/tamasmagyar/whisp/internal/stt"
	"github.com/tamasmagyar/whisp/internal/tray"
)

const usage = `usage: whisp [flags] [command]

commands:
  (none)                 start whisp in the background (tray icon in the top bar)
  stop                   stop the running instance
  meeting                start/stop meeting recording in the running instance (same as the meeting key)
  status                 report whether whisp is running
  download-model [name]  download a whisper.cpp model (default from config)
  listen [seconds]       record from the mic and print the transcript (test, no hotkey needed)

flags:
`

func main() {
	foreground := flag.Bool("foreground", false, "run attached to the terminal, logging to stderr")
	dryRun := flag.Bool("dry-run", false, "print transcripts instead of typing them")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage); flag.PrintDefaults() }
	flag.Parse()

	cfg, cfgPath, err := config.Load()
	if err != nil {
		fatal("config %s: %v", cfgPath, err)
	}

	switch flag.Arg(0) {
	case "":
	case "stop":
		if err := stopRunning(); err != nil {
			fatal("%v", err)
		}
		return
	case "meeting":
		pid := runningPID()
		if pid == 0 {
			fatal("whisp is not running")
		}
		if err := syscall.Kill(pid, syscall.SIGUSR1); err != nil {
			fatal("%v", err)
		}
		return
	case "status":
		if pid := runningPID(); pid != 0 {
			fmt.Printf("whisp is running (pid %d)\n", pid)
		} else {
			fmt.Println("whisp is not running")
			os.Exit(1)
		}
		return
	case "download-model":
		name := cfg.Model
		if flag.NArg() > 1 {
			name = flag.Arg(1)
		}
		if err := downloadModel(name); err != nil {
			fatal("%v", err)
		}
		return
	case "listen":
		secs := 4
		if flag.NArg() > 1 {
			fmt.Sscan(flag.Arg(1), &secs)
		}
		if err := listen(cfg, time.Duration(secs)*time.Second); err != nil {
			fatal("%v", err)
		}
		return
	default:
		flag.Usage()
		os.Exit(2)
	}

	if pid := runningPID(); pid != 0 {
		fatal("whisp is already running (pid %d); use 'whisp stop'", pid)
	}
	modelPath := config.ModelPath(cfg.Model)
	if _, err := os.Stat(modelPath); err != nil {
		if filepath.IsAbs(cfg.Model) {
			fatal("model not found: %s", modelPath)
		}
		fmt.Println("model not found, downloading it first")
		if err := downloadModel(cfg.Model); err != nil {
			fatal("%v", err)
		}
	}
	if !*foreground {
		if err := daemonize(config.LogPath()); err != nil {
			fatal("%v", err)
		}
		return
	}

	lock, err := acquireLock()
	if err != nil {
		fatal("%v", err)
	}
	defer lock.Close()
	run(cfg, modelPath, *dryRun)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "whisp: "+format+"\n", args...)
	os.Exit(1)
}

func run(cfg config.Config, modelPath string, dryRun bool) {
	log.SetFlags(log.Ldate | log.Ltime)
	log.Printf("starting (model %s, dictation %s, meeting %s, dry-run %v)", filepath.Base(modelPath), cfg.Key, cfg.MeetingKey, dryRun)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Print("signal received, quitting")
		tray.Quit()
	}()

	title := fmt.Sprintf("Hold %s to dictate · %s records meetings", keyLabel(cfg.Key), keyLabel(cfg.MeetingKey))
	stop := make(chan struct{})
	done := make(chan struct{})
	toggle := make(chan os.Signal, 1)
	signal.Notify(toggle, syscall.SIGUSR1)
	tray.Run(title, func(t *tray.Tray) {
		go func() {
			defer close(done)
			if err := serve(t, cfg, modelPath, dryRun, stop, toggle); err != nil {
				log.Print("error: ", err)
				t.Set(tray.Error, "Error: "+err.Error())
			}
		}()
	}, func() {
		// Give a running meeting recording time to transcribe its last chunk
		// and write the footer.
		close(stop)
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			log.Print("timed out finishing pending work")
		}
		log.Print("bye")
	})
}

func keyLabel(k string) string {
	if len(k) > 4 && k[:4] == "KEY_" {
		return k[4:]
	}
	return k
}

// serve runs the main loop until stop is closed. A value on toggleMeeting
// starts/stops meeting recording like the meeting key.
func serve(t *tray.Tray, cfg config.Config, modelPath string, dryRun bool, stop <-chan struct{}, toggleMeeting <-chan os.Signal) error {
	dictKey, err := hotkey.ParseKey(cfg.Key)
	if err != nil {
		return err
	}
	meetKey, err := hotkey.ParseKey(cfg.MeetingKey)
	if err != nil {
		return err
	}
	meetDir := config.ExpandHome(cfg.MeetingsDir)
	if !meeting.ValidTimestampMode(cfg.MeetingTimestamps) {
		return fmt.Errorf("meeting_timestamps must be clock, elapsed or none, not %q", cfg.MeetingTimestamps)
	}

	var out *inject.Injector
	if !dryRun {
		if out, err = inject.New(cfg.Paste, cfg.RestoreClipboard); err != nil {
			return err
		}
	}
	keys, err := hotkey.Watch(dictKey, meetKey)
	if err != nil {
		return err
	}
	rec, err := audio.New(cfg.Mic)
	if err != nil {
		return err
	}

	t.Set(tray.Loading, "")
	start := time.Now()
	engine, err := stt.Load(modelPath, cfg.Language, cfg.Threads, cfg.Prompt)
	if err != nil {
		return err
	}
	log.Printf("model loaded in %v", time.Since(start).Round(time.Millisecond))

	// A job is a dictation clip to paste, a meeting chunk to append to the
	// transcript file, or a literal line (header/footer) for that file.
	// renameTo, set on the footer, gives the transcript its final name.
	// Meeting chunks carry the recording's start and their offset into it,
	// for the sentence timestamps.
	type job struct {
		samples  []float32
		meeting  bool
		file     string
		line     string
		renameTo string
		recStart time.Time
		offset   time.Duration
	}
	jobs := make(chan job, 64)
	state := make(chan tray.State, 64)
	workerDone := make(chan struct{})

	process := func(j job) error {
		if j.line != "" {
			if err := meeting.Append(j.file, j.line); err != nil {
				return err
			}
			if j.renameTo != "" {
				if err := os.Rename(j.file, j.renameTo); err != nil {
					return err
				}
				log.Printf("meeting: saved %s", j.renameTo)
			}
			return nil
		}
		start := time.Now()
		audioLen := time.Duration(len(j.samples)) * time.Second / audio.SampleRate
		if j.meeting && cfg.MeetingTimestamps != "none" {
			segs, err := engine.Segments(j.samples)
			if err != nil {
				return fmt.Errorf("transcribe: %w", err)
			}
			lines := make([]string, len(segs))
			for i, seg := range segs {
				lines[i] = meeting.Timestamp(cfg.MeetingTimestamps, j.recStart, j.offset+seg.Start) + " " + seg.Text
			}
			log.Printf("meeting: transcribed %v of audio in %v (%d sentences)", audioLen.Round(10*time.Millisecond), time.Since(start).Round(time.Millisecond), len(lines))
			if len(lines) == 0 {
				return nil
			}
			return meeting.Append(j.file, strings.Join(lines, "\n"))
		}
		text, err := engine.Transcribe(j.samples)
		if err != nil {
			return fmt.Errorf("transcribe: %w", err)
		}
		took := time.Since(start).Round(time.Millisecond)
		if j.meeting {
			log.Printf("meeting: transcribed %v of audio in %v (%d chars)", audioLen.Round(10*time.Millisecond), took, len(text))
			if text == "" {
				return nil
			}
			return meeting.Append(j.file, text)
		}
		log.Printf("transcribed %v of audio in %v: %q", audioLen.Round(10*time.Millisecond), took, text)
		if text == "" {
			return nil
		}
		if cfg.TrailingSpace {
			text += " "
		}
		if dryRun {
			fmt.Println(text)
			return nil
		}
		return out.Type(text)
	}

	// Worker: processes jobs in order, so pastes and transcript lines keep their order.
	go func() {
		defer close(workerDone)
		for j := range jobs {
			state <- tray.Transcribing
			if err := process(j); err != nil {
				log.Print(err)
				state <- tray.Error
				continue
			}
			state <- tray.Idle
		}
	}()

	// Meeting recording state; nil while off.
	type session struct {
		start   time.Time
		file    string // transcript, named <start>_recording.txt until stopped
		monitor *audio.Recorder
		mixer   meeting.Mixer
		chunker meeting.Chunker
		emitted int // samples handed out as chunks so far (incl. skipped ones)
	}
	var mtg *session

	recording := false // dictation key held
	pending := 0
	current := tray.Idle
	show := func() {
		switch {
		case recording:
			t.Set(tray.Recording, "")
		case mtg != nil && current != tray.Error:
			t.Set(tray.Meeting, "Recording meeting → "+cfg.MeetingsDir)
		default:
			t.Set(current, "")
		}
	}
	enqueue := func(j job) {
		pending++
		jobs <- j
	}
	// chunk queues a meeting chunk for transcription unless it is silent.
	chunk := func(c []float32) {
		start := mtg.emitted
		mtg.emitted += len(c)
		if !meeting.HasSpeech(c) {
			return
		}
		c, cut := meeting.TrimLeadingSilence(c)
		offset := time.Duration(start+cut) * time.Second / audio.SampleRate
		enqueue(job{samples: c, meeting: true, file: mtg.file, recStart: mtg.start, offset: offset})
	}
	feed := func(samples []float32) {
		for _, c := range mtg.chunker.Add(samples) {
			chunk(c)
		}
	}
	startMeeting := func() {
		if recording { // F10 pressed while dictating: drop the dictation clip
			recording = false
			rec.Stop()
		}
		mon, err := audio.NewMonitor()
		if err != nil {
			log.Print("meeting: system audio: ", err)
			current = tray.Error
			t.Set(tray.Error, "Error: cannot record system audio")
			return
		}
		if err := os.MkdirAll(meetDir, 0o700); err != nil {
			mon.Close()
			log.Print("meeting: ", err)
			current = tray.Error
			t.Set(tray.Error, "Error: cannot create "+cfg.MeetingsDir)
			return
		}
		start := time.Now()
		mtg = &session{start: start, file: filepath.Join(meetDir, meeting.InProgressName(start)), monitor: mon}
		rec.Start()
		mon.Start()
		enqueue(job{file: mtg.file, line: meeting.StartedHeader(start)})
		log.Printf("meeting: recording started, writing to %s", mtg.file)
	}
	stopMeeting := func() {
		feed(mtg.mixer.Add(rec.Stop(), mtg.monitor.Stop()))
		feed(mtg.mixer.Flush())
		chunk(mtg.chunker.Flush())
		end := time.Now()
		final := filepath.Join(meetDir, meeting.FileName(mtg.start, end))
		enqueue(job{file: mtg.file, line: meeting.StoppedFooter(end), renameTo: final})
		mtg.monitor.Close()
		mtg = nil
		log.Print("meeting: recording stopped")
	}

	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	t.Set(tray.Idle, "")
	for {
		select {
		case <-stop:
			// Quitting: finish the meeting transcript (and footer) before exiting.
			if mtg != nil {
				stopMeeting()
			}
			close(jobs)
			<-workerDone
			return nil

		case <-toggleMeeting:
			if mtg == nil {
				startMeeting()
			} else {
				stopMeeting()
			}
			show()

		case ev := <-keys.Events():
			switch {
			case ev.Key == meetKey:
				if !ev.Down {
					break
				}
				if mtg == nil {
					startMeeting()
				} else {
					stopMeeting()
				}
			case mtg != nil:
				// Dictation is disabled while recording a meeting.
			case ev.Down && !recording:
				recording = true
				rec.Start()
			case !ev.Down && recording:
				recording = false
				samples := rec.Stop()
				dur := time.Duration(len(samples)) * time.Second / audio.SampleRate
				if dur < cfg.MinDuration.Duration {
					log.Printf("ignored %v clip", dur.Round(time.Millisecond))
				} else {
					enqueue(job{samples: samples})
				}
			}
			show()

		case <-tick.C:
			if mtg != nil {
				feed(mtg.mixer.Add(rec.Take(), mtg.monitor.Take()))
			}

		case s := <-state:
			if s != tray.Transcribing {
				pending--
			}
			if pending > 0 && s == tray.Idle {
				s = tray.Transcribing
			}
			current = s
			show()
		}
	}
}

func listen(cfg config.Config, d time.Duration) error {
	engine, err := stt.Load(config.ModelPath(cfg.Model), cfg.Language, cfg.Threads, cfg.Prompt)
	if err != nil {
		return err
	}
	defer engine.Close()
	rec, err := audio.New(cfg.Mic)
	if err != nil {
		return err
	}
	defer rec.Close()
	fmt.Fprintf(os.Stderr, "speak now (%v)...\n", d)
	rec.Start()
	time.Sleep(d)
	samples := rec.Stop()
	start := time.Now()
	text, err := engine.Transcribe(samples)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "(%d samples, transcribed in %v)\n", len(samples), time.Since(start).Round(time.Millisecond))
	fmt.Println(text)
	return nil
}
