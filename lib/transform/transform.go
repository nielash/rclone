// Package transform holds functions for path name transformations
package transform

import (
	"errors"
	"mime"
	"os"
	"path"
	"strings"

	"github.com/rclone/rclone/fs"
)

// Path transforms a path s according to the --name-transform options in use
//
// If no transforms are in use, s is returned unchanged
func Path(s string, isDir bool) string {
	if len(Opt.transforms) == 0 {
		return s
	}

	var err error
	old := s
	for _, t := range Opt.transforms {
		if isDir && t.tag == file {
			continue
		}
		baseOnly := !isDir && t.tag == file
		if t.tag == dir && !isDir {
			s, err = transformDir(s, t)
		} else {
			s, err = transformPath(s, t, baseOnly)
		}
		if err != nil {
			fs.Error(s, err.Error()) // TODO: return err instead of logging it?
		}
	}
	if old != s {
		fs.Debugf(old, "transformed to: %v", s)
	}
	return s
}

// transformPath transforms a path string according to the chosen TransformAlgo.
// Each path segment is transformed separately, to preserve path separators.
// If baseOnly is true, only the base will be transformed (useful for renaming while walking a dir tree recursively.)
// for example, "some/nested/path" -> "some/nested/CONVERTEDPATH"
// otherwise, the entire is path is transformed.
func transformPath(s string, t transform, baseOnly bool) (string, error) {
	if s == "" || s == "/" || s == "\\" || s == "." {
		return "", nil
	}

	if baseOnly {
		transformedBase, err := transformPathSegment(path.Base(s), t)
		return path.Join(path.Dir(s), transformedBase), err
	}

	segments := strings.Split(s, string(os.PathSeparator))
	transformedSegments := make([]string, len(segments))
	for _, seg := range segments {
		convSeg, err := transformPathSegment(seg, t)
		if err != nil {
			return "", err
		}
		transformedSegments = append(transformedSegments, convSeg)
	}
	return path.Join(transformedSegments...), nil
}

// transform all but the last path segment
func transformDir(s string, t transform) (string, error) {
	dirPath, err := transformPath(path.Dir(s), t, false)
	if err != nil {
		return "", err
	}
	return path.Join(dirPath, path.Base(s)), nil
}

// transformPathSegment transforms one path segment (or really any string) according to the chosen TransformAlgo.
// It assumes path separators have already been trimmed.
func transformPathSegment(s string, t transform) (string, error) {
	switch t.key {
	case ConvNone:
		return s, nil
		/* 	case ConvToNFC:
		   		return norm.NFC.String(s), nil
		   	case ConvToNFD:
		   		return norm.NFD.String(s), nil
		   	case ConvToNFKC:
		   		return norm.NFKC.String(s), nil
		   	case ConvToNFKD:
		   		return norm.NFKD.String(s), nil
		   	case ConvBase64Encode:
		   		return base64.URLEncoding.EncodeToString([]byte(s)), nil // URLEncoding to avoid slashes
		   	case ConvBase64Decode:
		   		if s == ".DS_Store" {
		   			return s, nil
		   		}
		   		b, err := base64.URLEncoding.DecodeString(s)
		   		return string(b), err
		   	case ConvFindReplace:
		   		oldNews := []string{}
		   		for _, pair := range Opt.FindReplace {
		   			split := strings.Split(pair, ",")
		   			oldNews = append(oldNews, split...)
		   		}
		   		replacer := strings.NewReplacer(oldNews...)
		   		return replacer.Replace(s), nil */
	case ConvPrefix:
		return t.value + s, nil
	case ConvSuffix:
		return s + t.value, nil
	case ConvSuffixKeepExtension:
		return SuffixKeepExtension(s, t.value), nil
	case ConvTrimPrefix:
		return strings.TrimPrefix(s, t.value), nil
	case ConvTrimSuffix:
		return strings.TrimSuffix(s, t.value), nil
		/*
			case ConvTruncate:
				if Opt.Max <= 0 {
					return s, nil
				}
				if utf8.RuneCountInString(s) <= Opt.Max {
					return s, nil
				}
				runes := []rune(s)
				return string(runes[:Opt.Max]), nil
			case ConvEncoder:
				return Opt.Enc.Encode(s), nil
			case ConvDecoder:
				return Opt.Enc.Decode(s), nil
			case ConvISO8859_1:
				return encodeWithReplacement(s, charmap.ISO8859_1), nil
			case ConvWindows1252:
				return encodeWithReplacement(s, charmap.Windows1252), nil
			case ConvMacintosh:
				return encodeWithReplacement(s, charmap.Macintosh), nil
			case ConvCharmap:
				return encodeWithReplacement(s, Opt.Cmap), nil
			case ConvLowercase:
				return strings.ToLower(s), nil
			case ConvUppercase:
				return strings.ToUpper(s), nil
			case ConvTitlecase:
				return strings.ToTitle(s), nil
			case ConvASCII:
				return toASCII(s), nil */
	default:
		return "", errors.New("this option is not yet implemented")
	}
}

// SuffixKeepExtension adds a suffix while keeping extension
//
// i.e. file.txt becomes file_somesuffix.txt not file.txt_somesuffix
func SuffixKeepExtension(remote string, suffix string) string {
	var (
		base  = remote
		exts  = ""
		first = true
		ext   = path.Ext(remote)
	)
	for ext != "" {
		// Look second and subsequent extensions in mime types.
		// If they aren't found then don't keep it as an extension.
		if !first && mime.TypeByExtension(ext) == "" {
			break
		}
		base = base[:len(base)-len(ext)]
		exts = ext + exts
		first = false
		ext = path.Ext(base)
	}
	return base + suffix + exts
}
