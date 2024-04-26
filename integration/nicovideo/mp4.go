package nicovideo

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Eyevinn/mp4ff/bits"
	"github.com/Eyevinn/mp4ff/mp4"
)

func combineInitSegments(files [][]byte, w io.Writer) (err error) {
	var combinedInit *mp4.InitSegment

	for i, data := range files {
		var f *mp4.File

		f, err = mp4.DecodeFileSR(bits.NewFixedSliceReader(data))
		if err != nil {
			err = fmt.Errorf("failed to decode init segment: %w", err)

			return
		}

		init := f.Init
		if len(init.Moov.Traks) != 1 {
			err = fmt.Errorf("expected exactly one track per init file")

			return
		}

		init.Moov.Trak.Tkhd.TrackID = uint32(i + 1)
		if init.Moov.Mvex != nil && init.Moov.Mvex.Trex != nil {
			init.Moov.Mvex.Trex.TrackID = uint32(i + 1)
		}

		if i == 0 {
			combinedInit = init

			continue
		}

		combinedInit.Moov.AddChild(init.Moov.Trak)

		if init.Moov.Mvex != nil {
			if init.Moov.Mvex.Trex != nil {
				combinedInit.Moov.Mvex.AddChild(init.Moov.Mvex.Trex)
			}

			if init.Moov.Mvex.Mehd != nil {
				combinedInit.Moov.Mvex.AddChild(init.Moov.Mvex.Mehd)
			}
		}
	}

	return combinedInit.Encode(w)
}

func combineMediaSegmentsUpdateSidx(seg *mp4.MediaSegment, frag *mp4.Fragment) {
	var total uint64

	for i := len(seg.Sidxs) - 1; i >= 0; i-- {
		seg.Sidxs[i].FirstOffset = total

		var refs []mp4.SidxRef

		for _, r := range seg.Sidxs[i].SidxRefs {
			r.ReferencedSize = uint32(frag.Size())

			refs = append(refs, r)
		}

		seg.Sidxs[i].SidxRefs = refs

		if total == 0 {
			total += seg.Sidxs[i].Size()

			continue
		}
	}
}

func combineMediaSegments(files [][]byte, w io.WriteCloser) error {
	var idx []uint32

	for i := range files {
		idx = append(idx, uint32(i+1))
	}

	var (
		outseg  *mp4.MediaSegment
		outfrag *mp4.Fragment
	)

	for i, data := range files {
		f, err := mp4.DecodeFileSR(bits.NewFixedSliceReader(data))
		if err != nil {
			return fmt.Errorf("failed to decode media segment: %w", err)
		}

		if len(f.Segments) != 1 {
			return fmt.Errorf("expected exactly one media segment per file")
		}

		inseg := f.Segments[0]

		if i == 0 {
			if inseg.Styp != nil {
				outseg = mp4.NewMediaSegmentWithStyp(inseg.Styp)
			} else {
				outseg = mp4.NewMediaSegmentWithoutStyp()
			}
		}

		for _, sidx := range inseg.Sidxs {
			sidx.ReferenceID = uint32(i + 1)
			outseg.AddSidx(sidx)
		}

		err = populateSegment(outseg, &outfrag, i, inseg, idx)
		if err != nil {
			return err
		}
	}

	combineMediaSegmentsUpdateSidx(outseg, outfrag)

	outseg.EncOptimize = mp4.OptimizeTrun

	return outseg.Encode(w)
}

func populateSegment(
	outseg *mp4.MediaSegment,
	outfrag **mp4.Fragment,
	i int,
	inseg *mp4.MediaSegment,
	idx []uint32,
) (err error) {
	for fi, infrag := range inseg.Fragments {
		if len(infrag.Moof.Trafs) != 1 {
			return fmt.Errorf("expected exactly one traf per fragment")
		}

		if i == 0 && fi == 0 {
			seqNr := infrag.Moof.Mfhd.SequenceNumber

			*outfrag, err = mp4.CreateMultiTrackFragment(seqNr, idx)
			if err != nil {
				return fmt.Errorf("failed to create fragment: %w", err)
			}

			outseg.AddFragment(*outfrag)
		}

		var fss []mp4.FullSample

		fss, err = infrag.GetFullSamples(nil)
		if err != nil {
			return fmt.Errorf("failed to get full samples: %w", err)
		}

		for _, fs := range fss {
			err = (*outfrag).AddFullSampleToTrack(fs, uint32(i+1))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type copyrange struct {
	Offset int64
	Length int64
}

type sampleoffset struct {
	mp4.Sample
	Offset int64
}

func defragmentMP4Trak(oldtrak *mp4.TrakBox) (newtrak *mp4.TrakBox, err error) {
	newtrak = mp4.NewTrakBox()

	var edtsfound bool

	for _, c := range oldtrak.GetChildren() {
		if c.Type() == "edts" {
			edtsfound = true

			break
		}
	}

	for _, c := range oldtrak.GetChildren() {
		switch c.Type() {
		case "tkhd":
			newtrak.AddChild(c)

			if !edtsfound {
				newtrak.AddChild(&mp4.EdtsBox{})
				newtrak.Edts.Elst = append(newtrak.Edts.Elst, &mp4.ElstBox{})
				newtrak.Edts.AddChild(newtrak.Edts.Elst[0])
			}
		case "edts":
			newtrak.AddChild(c)

			if len(newtrak.Edts.Elst) == 0 {
				newtrak.Edts.Elst = append(newtrak.Edts.Elst, &mp4.ElstBox{})
				newtrak.Edts.AddChild(newtrak.Edts.Elst[0])
			}
		case "mdia":
			newtrak.AddChild(mp4.NewMdiaBox())
		default:
			newtrak.AddChild(c)
		}
	}

	if newtrak.Tkhd == nil {
		return nil, fmt.Errorf("tkhd not found for trak %d", oldtrak.Tkhd.TrackID)
	}

	if newtrak.Mdia == nil {
		return nil, fmt.Errorf("mdia not found for trak %d", oldtrak.Tkhd.TrackID)
	}

	return
}

func defragmentMP4Udat(out *mp4.MoovBox, metadata map[string]string) {
	udta := &mp4.UdtaBox{}

	meta := mp4.CreateMetaBox(0, &mp4.HdlrBox{HandlerType: "mdir"})

	udta.AddChild(meta)

	ilst := &mp4.IlstBox{}

	for k, v := range metadata {
		box := mp4.NewGenericContainerBox(k)

		box.AddChild(&mp4.DataBox{Data: ([]byte)(v)})

		ilst.AddChild(box)
	}

	meta.AddChild(ilst)

	out.AddChild(udta)
}

func defragmentMP4Moov(in *mp4.MoovBox, metadata map[string]string) (out *mp4.MoovBox, err error) {
	out = mp4.NewMoovBox()
	out.AddChild(in.Mvhd)

	out.Mvhd.Timescale = 1000

	for i, oldtrak := range in.Traks {
		var newtrak *mp4.TrakBox

		newtrak, err = defragmentMP4Trak(oldtrak)
		if err != nil {
			return
		}

		for _, c := range oldtrak.Mdia.GetChildren() {
			switch c.Type() {
			case "minf":
				newtrak.Mdia.AddChild(mp4.NewMinfBox())
			default:
				newtrak.Mdia.AddChild(c)
			}
		}

		if newtrak.Mdia.Mdhd == nil {
			return nil, fmt.Errorf("mdhd not found for trak %d", oldtrak.Tkhd.TrackID)
		}

		if newtrak.Mdia.Minf == nil {
			return nil, fmt.Errorf("minf not found for trak %d", oldtrak.Tkhd.TrackID)
		}

		if newtrak.Mdia.Hdlr == nil {
			return nil, fmt.Errorf("hdlr not found for trak %d", oldtrak.Tkhd.TrackID)
		}

		for _, c := range oldtrak.Mdia.Minf.GetChildren() {
			switch c.Type() {
			case "stbl":
				newtrak.Mdia.Minf.AddChild(mp4.NewStblBox())
			default:
				newtrak.Mdia.Minf.AddChild(c)
			}
		}

		stbl := newtrak.Mdia.Minf.Stbl
		if stbl == nil {
			return nil, fmt.Errorf("stbl not found for trak %d", oldtrak.Tkhd.TrackID)
		}

		stbl.AddChild(oldtrak.Mdia.Minf.Stbl.Stsd)
		stbl.AddChild(&mp4.SttsBox{})

		if i == 0 {
			stbl.AddChild(&mp4.StssBox{})
		}

		if newtrak.Mdia.Hdlr.HandlerType == "vide" {
			stbl.AddChild(&mp4.CttsBox{})
		}

		stbl.AddChild(&mp4.StscBox{})
		stbl.AddChild(&mp4.StszBox{})
		stbl.AddChild(&mp4.StcoBox{})

		for _, c := range stbl.GetChildren() {
			switch c.Type() {
			case "stsd", "stts", "stss", "ctts", "stsc", "stsz", "stco":
			default:
				stbl.AddChild(c)
			}
		}

		out.AddChild(newtrak)
	}

	defragmentMP4Udat(out, metadata)

	return
}

func defragmentMP4Samples(
	frag *mp4.Fragment,
	traf *mp4.TrafBox,
	trak *mp4.TrakBox,
	trex *mp4.TrexBox,
) (samples []sampleoffset, err error) {
	for _, trun := range traf.Truns {
		trun.AddSampleDefaultValues(traf.Tfhd, trex)

		mdatoffset := frag.Moof.StartPos

		if traf.Tfhd.HasBaseDataOffset() {
			mdatoffset = traf.Tfhd.BaseDataOffset
		} else if traf.Tfhd.DefaultBaseIfMoof() {
			mdatoffset = frag.Moof.StartPos
		}

		if trun.HasDataOffset() {
			mdatoffset += uint64(trun.DataOffset)
		}

		if mdatoffset == 0 {
			mdatoffset = frag.Mdat.PayloadAbsoluteOffset()
		}

		for _, s := range trun.Samples {
			idx := len(trak.Mdia.Minf.Stbl.Stts.SampleTimeDelta) - 1

			if idx < 0 || trak.Mdia.Minf.Stbl.Stts.SampleTimeDelta[idx] != s.Dur {
				trak.Mdia.Minf.Stbl.Stts.SampleCount = append(trak.Mdia.Minf.Stbl.Stts.SampleCount, 1)
				trak.Mdia.Minf.Stbl.Stts.SampleTimeDelta = append(trak.Mdia.Minf.Stbl.Stts.SampleTimeDelta, s.Dur)
			} else {
				trak.Mdia.Minf.Stbl.Stts.SampleCount[idx]++
			}

			if trak.Mdia.Hdlr.HandlerType == "vide" {
				err = trak.Mdia.Minf.Stbl.Ctts.AddSampleCountsAndOffset(
					[]uint32{1},
					[]int32{s.CompositionTimeOffset},
				)
				if err != nil {
					return
				}
			}

			trak.Mdia.Minf.Stbl.Stsz.SampleNumber++
			trak.Mdia.Minf.Stbl.Stsz.SampleSize = append(trak.Mdia.Minf.Stbl.Stsz.SampleSize, s.Size)

			samples = append(samples, sampleoffset{
				Sample: s,
				Offset: int64(mdatoffset),
			})

			mdatoffset += uint64(s.Size)
		}
	}

	return
}

type defragmenter struct {
	durations  []time.Duration
	timescales []time.Duration
	target     []copyrange
	incomplete [][]copyrange
	complete   [][]copyrange
	chunkid    uint32
	offset     uint32
	buffering  bool
	init       bool
}

func (defrag *defragmenter) minMax(
	tracksamples [][]sampleoffset,
) (maxdur time.Duration, present bool) {
	var basedur time.Duration

	for trackid, samples := range tracksamples {
		if len(samples) == 0 {
			continue
		}

		present = true

		dur := time.Duration(samples[0].Dur) * defrag.timescales[trackid]

		if dur > maxdur {
			maxdur = dur
			basedur = defrag.durations[trackid]
		}
	}

	maxdur += basedur

	return
}

func (defrag *defragmenter) track(
	tracksamples [][]sampleoffset,
	trackid int,
	maxdur time.Duration,
) (targets []copyrange) {
	for idx, sample := range tracksamples[trackid] {
		dur := time.Duration(sample.Dur) * defrag.timescales[trackid]

		targets = append(targets, copyrange{
			Offset: sample.Offset,
			Length: int64(sample.Size),
		})

		defrag.durations[trackid] += dur

		if defrag.durations[trackid] >= maxdur || idx+1 == len(tracksamples[trackid]) {
			tracksamples[trackid] = tracksamples[trackid][idx+1:]

			break
		}
	}

	return
}

func (defrag *defragmenter) append(chunk [][]copyrange, traks []*mp4.TrakBox) (err error) {
	for trackid, samples := range chunk {
		var locoffset uint32

		for _, sample := range samples {
			lasttarget := len(defrag.target) - 1

			if lasttarget >= 0 && defrag.target[lasttarget].Offset+defrag.target[lasttarget].Length == sample.Offset {
				defrag.target[lasttarget].Length += sample.Length
			} else {
				defrag.target = append(defrag.target, sample)
			}

			locoffset += uint32(sample.Length)
		}

		samplescount := uint32(len(samples))

		stsc := traks[trackid].Mdia.Minf.Stbl.Stsc

		if len(stsc.Entries) == 0 || stsc.Entries[len(stsc.Entries)-1].SamplesPerChunk != samplescount {
			err = stsc.AddEntry(defrag.chunkid, samplescount, 1)
			if err != nil {
				return
			}
		}

		stco := traks[trackid].Mdia.Minf.Stbl.Stco

		stco.ChunkOffset = append(stco.ChunkOffset, defrag.offset)

		defrag.offset += locoffset

		chunk[trackid] = chunk[trackid][:0]
	}

	defrag.chunkid++

	return
}

func (defrag *defragmenter) next(tracksamples [][]sampleoffset, traks []*mp4.TrakBox, last bool) (err error) {
	var available int

	for {
		maxdur, present := defrag.minMax(tracksamples)
		if !present {
			break
		}

		available = 0

		for trackid := range tracksamples {
			targets := defrag.track(tracksamples, trackid, maxdur)

			if len(targets) == 0 {
				defrag.buffering = true

				continue
			}

			defrag.incomplete[trackid] = append(defrag.incomplete[trackid], targets...)

			available++
		}

		if available == len(tracksamples) {
			defrag.buffering = false

			if defrag.init {
				err = defrag.append(defrag.complete, traks)
				if err != nil {
					return
				}
			} else {
				defrag.init = true
			}

			for trackid := range tracksamples {
				defrag.complete[trackid] = append(defrag.complete[trackid][:0], defrag.incomplete[trackid]...)
				defrag.incomplete[trackid] = defrag.incomplete[trackid][:0]
			}
		}
	}

	finalize := defrag.buffering && last

	if finalize {
		for trackid := range tracksamples {
			defrag.complete[trackid] = append(defrag.complete[trackid], defrag.incomplete[trackid]...)
			defrag.incomplete[trackid] = defrag.incomplete[trackid][:0]
		}
	}

	if available == len(tracksamples) || finalize {
		err = defrag.append(defrag.complete, traks)
		if err != nil {
			return
		}

		defrag.init = false
	}

	return
}

func defragmentMP4(in *mp4.File, metadata map[string]string) (out *mp4.File, target []copyrange, err error) {
	out = mp4.NewFile()
	out.AddChild(mp4.NewFtyp("isom", 512, []string{"isom", "iso2", "avc1", "mp41"}), 0)

	moov, err := defragmentMP4Moov(in.Moov, metadata)
	if err != nil {
		return
	}

	out.Moov = moov
	out.Children = append(out.Children, moov)

	out.AddChild(&mp4.MdatBox{}, 0)

	defrag := defragmenter{
		durations:  make([]time.Duration, len(moov.Traks)),
		timescales: make([]time.Duration, len(moov.Traks)),
		chunkid:    1,
		target:     nil,
		complete:   make([][]copyrange, len(moov.Traks)),
		incomplete: make([][]copyrange, len(moov.Traks)),
		offset:     0,
	}

	for trakid := range moov.Traks {
		defrag.timescales[trakid] = time.Second / time.Duration(moov.Traks[trakid].Mdia.Mdhd.Timescale)
	}

	tracksamples := make([][]sampleoffset, len(moov.Traks))

	var totalsamples uint32

	for sidx, segm := range in.Segments {
		for fidx, frag := range segm.Fragments {
			for trackid, traf := range frag.Moof.Trafs {
				trak := moov.Traks[trackid]
				trex := in.Moov.Mvex.Trexs[trackid]

				var samples []sampleoffset

				samples, err = defragmentMP4Samples(frag, traf, trak, trex)
				if err != nil {
					return
				}

				tracksamples[trackid] = samples

				if trackid == 0 {
					trak.Mdia.Minf.Stbl.Stss.SampleNumber = append(
						trak.Mdia.Minf.Stbl.Stss.SampleNumber,
						totalsamples+1,
					)

					totalsamples += uint32(len(samples))
				}
			}

			err = defrag.next(tracksamples, moov.Traks, sidx+1 == len(in.Segments) && fidx+1 == len(segm.Fragments))
			if err != nil {
				return
			}
		}
	}

	for trackid, duration := range defrag.durations {
		moov.Traks[trackid].Tkhd.Duration = uint64(duration / time.Millisecond)
		moov.Traks[trackid].Mdia.Mdhd.Duration = uint64(duration / defrag.timescales[trackid])
		moov.Traks[trackid].Edts.Elst[0].Entries = []mp4.ElstEntry{
			{
				SegmentDuration:   moov.Traks[trackid].Tkhd.Duration,
				MediaTime:         0,
				MediaRateInteger:  1,
				MediaRateFraction: 0,
			},
		}

		if trackid == 0 {
			moov.Mvhd.Duration = moov.Traks[trackid].Tkhd.Duration
		}
	}

	mdatoffset := uint32(out.Ftyp.Size() + moov.Size() + 8)

	for _, trak := range moov.Traks {
		for idx := range trak.Mdia.Minf.Stbl.Stco.ChunkOffset {
			trak.Mdia.Minf.Stbl.Stco.ChunkOffset[idx] += mdatoffset
		}
	}

	return out, defrag.target, nil
}

// DefragmentMP4 defragments MP4 in src writing resulting progressive MP4 to dst using native OS copy
func DefragmentMP4(src, dst *os.File, metadata map[string]string) (err error) {
	in, err := mp4.DecodeFile(src, mp4.WithDecodeMode(mp4.DecModeLazyMdat))
	if err != nil {
		return
	}

	if !in.IsFragmented() {
		_, err = src.Seek(0, io.SeekStart)
		if err != nil {
			return
		}

		_, err = dst.ReadFrom(src)
		if err != nil {
			return
		}

		return
	}

	out, target, err := defragmentMP4(in, metadata)
	if err != nil {
		return
	}

	err = out.Encode(dst)
	if err != nil {
		return err
	}

	for _, r := range target {
		_, err = src.Seek(r.Offset, io.SeekStart)
		if err != nil {
			return err
		}

		_, err = dst.ReadFrom(io.LimitReader(src, r.Length))
		if err != nil {
			return err
		}
	}

	return nil
}
