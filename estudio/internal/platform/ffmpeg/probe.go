package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Info son los datos de un archivo de audio según ffprobe.
type Info struct {
	Duration   time.Duration
	Title      string
	Artist     string
	Album      string
	Genre      string
	Composer   string
	Year       string
	Codec      string
	SampleRate int
	Channels   int
	BitRate    int // bits/s
}

// Probe lee duración, formato y etiquetas de un archivo.
func (t Tools) Probe(ctx context.Context, path string) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := command(ctx, t.FFprobe, "-v", "error", "-print_format", "json", "-show_format", "-show_streams",
		"-select_streams", "a:0", path)
	stderr := &limitedBuffer{max: 2048}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		return Info{}, fmt.Errorf("ffprobe %s: %w %s", path, err, strings.TrimSpace(stderr.String()))
	}
	return parseProbe(out)
}

type probeDoc struct {
	Streams []struct {
		CodecName  string            `json:"codec_name"`
		SampleRate string            `json:"sample_rate"`
		Channels   int               `json:"channels"`
		Duration   string            `json:"duration"`
		Tags       map[string]string `json:"tags"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		BitRate  string            `json:"bit_rate"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

func parseProbe(b []byte) (Info, error) {
	var d probeDoc
	if err := json.Unmarshal(b, &d); err != nil {
		return Info{}, fmt.Errorf("ffprobe: json: %w", err)
	}
	if len(d.Streams) == 0 {
		return Info{}, fmt.Errorf("ffprobe: el archivo no tiene audio")
	}
	s := d.Streams[0]
	info := Info{Codec: s.CodecName, Channels: s.Channels}
	info.SampleRate, _ = strconv.Atoi(s.SampleRate)
	info.BitRate, _ = strconv.Atoi(d.Format.BitRate)
	dur := d.Format.Duration
	if dur == "" {
		dur = s.Duration
	}
	if f, err := strconv.ParseFloat(dur, 64); err == nil {
		info.Duration = time.Duration(f * float64(time.Second))
	}
	// Las etiquetas pueden venir en el formato o en el stream, con mayúsculas variables.
	tags := map[string]string{}
	for k, v := range s.Tags {
		tags[strings.ToLower(k)] = v
	}
	for k, v := range d.Format.Tags {
		tags[strings.ToLower(k)] = v
	}
	info.Title = strings.TrimSpace(tags["title"])
	info.Artist = strings.TrimSpace(firstNonEmpty(tags["artist"], tags["album_artist"], tags["performer"]))
	info.Album = strings.TrimSpace(tags["album"])
	info.Genre = strings.TrimSpace(tags["genre"])
	info.Composer = strings.TrimSpace(tags["composer"])
	info.Year = strings.TrimSpace(firstNonEmpty(tags["date"], tags["year"]))
	if len(info.Year) > 4 {
		info.Year = info.Year[:4]
	}
	return info, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
