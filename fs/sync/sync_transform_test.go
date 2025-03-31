// Test transform

package sync

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	_ "github.com/rclone/rclone/backend/all"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/filter"
	"github.com/rclone/rclone/fs/operations"
	"github.com/rclone/rclone/fs/walk"
	"github.com/rclone/rclone/fstest"
	"github.com/rclone/rclone/lib/transform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"
)

// Some times used in the tests
var (
	debug = ``
)

func TestTransform(t *testing.T) {
	type args struct {
		TransformOpt     transform.Options
		TransformBackOpt transform.Options
		Lossless         bool // whether the TransformBackAlgo is always losslessly invertible
		// ExtraOpt          transform
	}
	tests := []struct {
		name string
		args args
	}{
		// {name: "NFC", args: args{TransformAlgo: ConvToNFC, TransformBackAlgo: ConvToNFD, Lossless: false}},
		// {name: "NFD", args: args{TransformAlgo: ConvToNFD, TransformBackAlgo: ConvToNFC, Lossless: false}},
		// {name: "NFKC", args: args{TransformAlgo: ConvToNFKC, TransformBackAlgo: ConvToNFKD, Lossless: false}},
		// {name: "NFKD", args: args{TransformAlgo: ConvToNFKD, TransformBackAlgo: ConvToNFKC, Lossless: false}},
		// {name: "base64", args: args{TransformAlgo: ConvBase64Encode, TransformBackAlgo: ConvBase64Decode, Lossless: true}},
		// {name: "replace", args: args{TransformAlgo: ConvFindReplace, TransformBackAlgo: ConvFindReplace, Lossless: true, ExtraOpt: transform{FindReplace: []string{"bread,banana", "pie,apple", "apple,pie", "banana,bread"}}}},
		{name: "prefix", args: args{
			TransformOpt:     transform.Options{Flags: transform.Flags{NameTransform: []string{"prefix=PREFIX"}}},
			TransformBackOpt: transform.Options{Flags: transform.Flags{NameTransform: []string{"trimprefix=PREFIX"}}},
		}},
		{name: "suffix", args: args{
			TransformOpt:     transform.Options{Flags: transform.Flags{NameTransform: []string{"suffix=SUFFIX"}}},
			TransformBackOpt: transform.Options{Flags: transform.Flags{NameTransform: []string{"trimsuffix=SUFFIX"}}},
		}},
		// {name: "truncate", args: args{TransformAlgo: ConvTruncate, TransformBackAlgo: ConvTruncate, Lossless: false, ExtraOpt: transform{value: "10"}}},
		// {name: "encoder", args: args{TransformAlgo: ConvEncoder, TransformBackAlgo: ConvDecoder, Lossless: true, ExtraOpt: transform{Enc: encoder.OS}}},
		// {name: "ISO-8859-1", args: args{TransformAlgo: ConvISO8859_1, TransformBackAlgo: ConvISO8859_1, Lossless: false}},
		// {name: "charmap", args: args{TransformAlgo: ConvCharmap, TransformBackAlgo: ConvCharmap, Lossless: false, ExtraOpt: transform{CmapFlag: 3}}},
		// {name: "lowercase", args: args{TransformAlgo: ConvLowercase, TransformBackAlgo: ConvUppercase, Lossless: false}},
		// {name: "ascii", args: args{TransformAlgo: ConvASCII, TransformBackAlgo: ConvASCII, Lossless: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := fstest.NewRun(t)
			defer r.Finalise()

			r.Mkdir(context.Background(), r.Flocal)
			r.Mkdir(context.Background(), r.Fremote)
			items := makeTestFiles(t, r, "dir1")
			deleteDSStore(t, r)
			r.CheckRemoteListing(t, items, nil)
			r.CheckLocalListing(t, items, nil)

			transform.Opt = tt.args.TransformOpt
			err := transform.Reload(context.Background())
			require.NoError(t, err)

			err = Sync(context.Background(), r.Fremote, r.Flocal, true)
			assert.NoError(t, err)
			compareNames(t, r, items)

			transformedItems := transformItems(t, items)
			transform.Opt = tt.args.TransformBackOpt
			err = transform.Reload(context.Background())
			require.NoError(t, err)
			err = Sync(context.Background(), r.Fremote, r.Flocal, true)
			assert.NoError(t, err)
			compareNames(t, r, transformedItems)

			if tt.args.Lossless {
				deleteDSStore(t, r)
				r.CheckRemoteItems(t, items...)
			}
		})
	}
}

// const alphabet = "ƀɀɠʀҠԀڀڠݠހ߀ကႠᄀᄠᅀᆀᇠሀሠበዠጠᎠᏀᐠᑀᑠᒀᒠᓀᓠᔀᔠᕀᕠᖀᖠᗀᗠᘀᘠᙀᚠᛀកᠠᡀᣀᦀ᧠ᨠᯀᰀᴀ⇠⋀⍀⍠⎀⎠⏀␀─┠╀╠▀■◀◠☀☠♀♠⚀⚠⛀⛠✀✠❀➀➠⠀⠠⡀⡠⢀⢠⣀⣠⤀⤠⥀⥠⦠⨠⩀⪀⪠⫠⬀⬠⭀ⰀⲀⲠⳀⴀⵀ⺠⻀㇀㐀㐠㑀㑠㒀㒠㓀㓠㔀㔠㕀㕠㖀㖠㗀㗠㘀㘠㙀㙠㚀㚠㛀㛠㜀㜠㝀㝠㞀㞠㟀㟠㠀㠠㡀㡠㢀㢠㣀㣠㤀㤠㥀㥠㦀㦠㧀㧠㨀㨠㩀㩠㪀㪠㫀㫠㬀㬠㭀㭠㮀㮠㯀㯠㰀㰠㱀㱠㲀㲠㳀㳠㴀㴠㵀㵠㶀㶠㷀㷠㸀㸠㹀㹠㺀㺠㻀㻠㼀㼠㽀㽠㾀㾠㿀㿠䀀䀠䁀䁠䂀䂠䃀䃠䄀䄠䅀䅠䆀䆠䇀䇠䈀䈠䉀䉠䊀䊠䋀䋠䌀䌠䍀䍠䎀䎠䏀䏠䐀䐠䑀䑠䒀䒠䓀䓠䔀䔠䕀䕠䖀䖠䗀䗠䘀䘠䙀䙠䚀䚠䛀䛠䜀䜠䝀䝠䞀䞠䟀䟠䠀䠠䡀䡠䢀䢠䣀䣠䤀䤠䥀䥠䦀䦠䧀䧠䨀䨠䩀䩠䪀䪠䫀䫠䬀䬠䭀䭠䮀䮠䯀䯠䰀䰠䱀䱠䲀䲠䳀䳠䴀䴠䵀䵠䶀䷀䷠一丠乀习亀亠什仠伀传佀你侀侠俀俠倀倠偀偠傀傠僀僠儀儠兀兠冀冠净几刀删剀剠劀加勀勠匀匠區占厀厠叀叠吀吠呀呠咀咠哀哠唀唠啀啠喀喠嗀嗠嘀嘠噀噠嚀嚠囀因圀圠址坠垀垠埀埠堀堠塀塠墀墠壀壠夀夠奀奠妀妠姀姠娀娠婀婠媀媠嫀嫠嬀嬠孀孠宀宠寀寠尀尠局屠岀岠峀峠崀崠嵀嵠嶀嶠巀巠帀帠幀幠庀庠廀廠开张彀彠往徠忀忠怀怠恀恠悀悠惀惠愀愠慀慠憀憠懀懠戀戠所扠技抠拀拠挀挠捀捠掀掠揀揠搀搠摀摠撀撠擀擠攀攠敀敠斀斠旀无昀映晀晠暀暠曀曠最朠杀杠枀枠柀柠栀栠桀桠梀梠检棠椀椠楀楠榀榠槀槠樀樠橀橠檀檠櫀櫠欀欠歀歠殀殠毀毠氀氠汀池沀沠泀泠洀洠浀浠涀涠淀淠渀渠湀湠満溠滀滠漀漠潀潠澀澠激濠瀀瀠灀灠炀炠烀烠焀焠煀煠熀熠燀燠爀爠牀牠犀犠狀狠猀猠獀獠玀玠珀珠琀琠瑀瑠璀璠瓀瓠甀甠畀畠疀疠痀痠瘀瘠癀癠皀皠盀盠眀眠着睠瞀瞠矀矠砀砠础硠碀碠磀磠礀礠祀祠禀禠秀秠稀稠穀穠窀窠竀章笀笠筀筠简箠節篠簀簠籀籠粀粠糀糠紀素絀絠綀綠緀締縀縠繀繠纀纠绀绠缀缠罀罠羀羠翀翠耀耠聀聠肀肠胀胠脀脠腀腠膀膠臀臠舀舠艀艠芀芠苀苠茀茠荀荠莀莠菀菠萀萠葀葠蒀蒠蓀蓠蔀蔠蕀蕠薀薠藀藠蘀蘠虀虠蚀蚠蛀蛠蜀蜠蝀蝠螀螠蟀蟠蠀蠠血衠袀袠裀裠褀褠襀襠覀覠觀觠言訠詀詠誀誠諀諠謀謠譀譠讀讠诀诠谀谠豀豠貀負賀賠贀贠赀赠趀趠跀跠踀踠蹀蹠躀躠軀軠輀輠轀轠辀辠迀迠退造遀遠邀邠郀郠鄀鄠酀酠醀醠釀釠鈀鈠鉀鉠銀銠鋀鋠錀錠鍀鍠鎀鎠鏀鏠鐀鐠鑀鑠钀钠铀铠销锠镀镠門閠闀闠阀阠陀陠隀隠雀雠需霠靀靠鞀鞠韀韠頀頠顀顠颀颠飀飠餀餠饀饠馀馠駀駠騀騠驀驠骀骠髀髠鬀鬠魀魠鮀鮠鯀鯠鰀鰠鱀鱠鲀鲠鳀鳠鴀鴠鵀鵠鶀鶠鷀鷠鸀鸠鹀鹠麀麠黀黠鼀鼠齀齠龀龠ꀀꀠꁀꁠꂀꂠꃀꃠꄀꄠꅀꅠꆀꆠꇀꇠꈀꈠꉀꉠꊀꊠꋀꋠꌀꌠꍀꍠꎀꎠꏀꏠꐀꐠꑀꑠ꒠ꔀꔠꕀꕠꖀꖠꗀꗠꙀꚠꛀ꜀꜠ꝀꞀꡀ測試_Русский___ě_áñ"
const alphabet = "abcdefg123456789"

var extras = []string{"apple", "banana", "appleappleapplebanana", "splitbananasplit"}

func makeTestFiles(t *testing.T, r *fstest.Run, dir string) []fstest.Item {
	t.Helper()
	n := 0
	// Create test files
	items := []fstest.Item{}
	for _, c := range alphabet {
		var out strings.Builder
		for i := rune(0); i < 7; i++ {
			out.WriteRune(c + i)
		}
		fileName := filepath.Join(dir, fmt.Sprintf("%04d-%s.txt", n, out.String()))
		fileName = strings.ToValidUTF8(fileName, "")

		if debug != "" {
			fileName = debug
		}

		item := r.WriteObject(context.Background(), fileName, fileName, t1)
		r.WriteFile(fileName, fileName, t1)
		items = append(items, item)
		n++

		if debug != "" {
			break
		}
	}

	for _, extra := range extras {
		item := r.WriteObject(context.Background(), extra, extra, t1)
		r.WriteFile(extra, extra, t1)
		items = append(items, item)
	}

	return items
}

func deleteDSStore(t *testing.T, r *fstest.Run) {
	ctxDSStore, fi := filter.AddConfig(context.Background())
	err := fi.AddRule(`+ *.DS_Store`)
	assert.NoError(t, err)
	err = fi.AddRule(`- **`)
	assert.NoError(t, err)
	err = operations.Delete(ctxDSStore, r.Fremote)
	assert.NoError(t, err)
}

func compareNames(t *testing.T, r *fstest.Run, items []fstest.Item) {
	var entries fs.DirEntries

	deleteDSStore(t, r)
	err := walk.ListR(context.Background(), r.Fremote, "", true, -1, walk.ListObjects, func(e fs.DirEntries) error {
		entries = append(entries, e...)
		return nil
	})
	assert.NoError(t, err)
	entries = slices.DeleteFunc(entries, func(E fs.DirEntry) bool { // remove those pesky .DS_Store files
		if strings.Contains(E.Remote(), ".DS_Store") {
			err := operations.DeleteFile(context.Background(), E.(fs.Object))
			assert.NoError(t, err)
			return true
		}
		return false
	})
	require.Equal(t, len(items), entries.Len())

	// sort by CONVERTED name
	slices.SortStableFunc(items, func(a, b fstest.Item) int {
		aConv := transform.Path(a.Path, false)
		bConv := transform.Path(b.Path, false)
		return cmp.Compare(aConv, bConv)
	})
	slices.SortStableFunc(entries, func(a, b fs.DirEntry) int {
		return cmp.Compare(a.Remote(), b.Remote())
	})

	for i, e := range entries {
		expect := transform.Path(items[i].Path, false)
		msg := fmt.Sprintf("expected %v, got %v", detectEncoding(expect), detectEncoding(e.Remote()))
		assert.Equal(t, expect, e.Remote(), msg)
	}
}

func transformItems(t *testing.T, items []fstest.Item) []fstest.Item {
	transformedItems := []fstest.Item{}
	for _, item := range items {
		newPath := transform.Path(item.Path, false)
		newItem := item
		newItem.Path = newPath
		transformedItems = append(transformedItems, newItem)
	}
	return transformedItems
}

func detectEncoding(s string) string {
	if norm.NFC.IsNormalString(s) && norm.NFD.IsNormalString(s) {
		return "BOTH"
	}
	if !norm.NFC.IsNormalString(s) && norm.NFD.IsNormalString(s) {
		return "NFD"
	}
	if norm.NFC.IsNormalString(s) && !norm.NFD.IsNormalString(s) {
		return "NFC"
	}
	return "OTHER"
}

func TestTransformCopy(t *testing.T) {
	ctx := context.Background()
	r := fstest.NewRun(t)
	transform.Opt = transform.Options{Flags: transform.Flags{NameTransform: []string{"all,suffix_keep_extension=_somesuffix"}}}
	err := transform.Reload(ctx)
	require.NoError(t, err)
	file1 := r.WriteFile("sub dir/hello world.txt", "hello world", t1)

	r.Mkdir(ctx, r.Fremote)
	ctx = predictDstFromLogger(ctx)
	err = Sync(ctx, r.Fremote, r.Flocal, true)
	testLoggerVsLsf(ctx, r.Fremote, operations.GetLoggerOpt(ctx).JSON, t)
	require.NoError(t, err)

	r.CheckLocalItems(t, file1)
	r.CheckRemoteItems(t, fstest.NewItem("sub dir_somesuffix/hello world_somesuffix.txt", "hello world", t1))
}

func TestDoubleTransform(t *testing.T) {
	ctx := context.Background()
	r := fstest.NewRun(t)
	transform.Opt = transform.Options{Flags: transform.Flags{NameTransform: []string{"all,prefix=tac", "all,prefix=tic"}}}
	err := transform.Reload(ctx)
	require.NoError(t, err)
	file1 := r.WriteFile("toe/toe", "hello world", t1)

	r.Mkdir(ctx, r.Fremote)
	ctx = predictDstFromLogger(ctx)
	err = Sync(ctx, r.Fremote, r.Flocal, true)
	testLoggerVsLsf(ctx, r.Fremote, operations.GetLoggerOpt(ctx).JSON, t)
	require.NoError(t, err)

	r.CheckLocalItems(t, file1)
	r.CheckRemoteItems(t, fstest.NewItem("tictactoe/tictactoe", "hello world", t1))
}

func TestFileTag(t *testing.T) {
	ctx := context.Background()
	r := fstest.NewRun(t)
	transform.Opt = transform.Options{Flags: transform.Flags{NameTransform: []string{"file,prefix=tac", "file,prefix=tic"}}}
	err := transform.Reload(ctx)
	require.NoError(t, err)
	file1 := r.WriteFile("toe/toe/toe", "hello world", t1)

	r.Mkdir(ctx, r.Fremote)
	ctx = predictDstFromLogger(ctx)
	err = Sync(ctx, r.Fremote, r.Flocal, true)
	testLoggerVsLsf(ctx, r.Fremote, operations.GetLoggerOpt(ctx).JSON, t)
	require.NoError(t, err)

	r.CheckLocalItems(t, file1)
	r.CheckRemoteItems(t, fstest.NewItem("toe/toe/tictactoe", "hello world", t1))
}

func TestNoTag(t *testing.T) {
	ctx := context.Background()
	r := fstest.NewRun(t)
	transform.Opt = transform.Options{Flags: transform.Flags{NameTransform: []string{"prefix=tac", "prefix=tic"}}}
	err := transform.Reload(ctx)
	require.NoError(t, err)
	file1 := r.WriteFile("toe/toe/toe", "hello world", t1)

	r.Mkdir(ctx, r.Fremote)
	ctx = predictDstFromLogger(ctx)
	err = Sync(ctx, r.Fremote, r.Flocal, true)
	testLoggerVsLsf(ctx, r.Fremote, operations.GetLoggerOpt(ctx).JSON, t)
	require.NoError(t, err)

	r.CheckLocalItems(t, file1)
	r.CheckRemoteItems(t, fstest.NewItem("toe/toe/tictactoe", "hello world", t1))
}

func TestDirTag(t *testing.T) {
	ctx := context.Background()
	r := fstest.NewRun(t)
	transform.Opt = transform.Options{Flags: transform.Flags{NameTransform: []string{"dir,prefix=tac", "dir,prefix=tic"}}}
	err := transform.Reload(ctx)
	require.NoError(t, err)
	file1 := r.WriteFile("toe/toe/toe.txt", "hello world", t1)

	r.Mkdir(ctx, r.Fremote)
	ctx = predictDstFromLogger(ctx)
	err = Sync(ctx, r.Fremote, r.Flocal, true)
	testLoggerVsLsf(ctx, r.Fremote, operations.GetLoggerOpt(ctx).JSON, t)
	require.NoError(t, err)

	r.CheckLocalItems(t, file1)
	r.CheckRemoteItems(t, fstest.NewItem("tictactoe/tictactoe/toe.txt", "hello world", t1))
}

func TestRunTwice(t *testing.T) {
	ctx := context.Background()
	r := fstest.NewRun(t)
	transform.Opt = transform.Options{Flags: transform.Flags{NameTransform: []string{"dir,prefix=tac", "dir,prefix=tic"}}}
	err := transform.Reload(ctx)
	require.NoError(t, err)
	file1 := r.WriteFile("toe/toe/toe.txt", "hello world", t1)

	r.Mkdir(ctx, r.Fremote)
	ctx = predictDstFromLogger(ctx)
	err = Sync(ctx, r.Fremote, r.Flocal, true)
	testLoggerVsLsf(ctx, r.Fremote, operations.GetLoggerOpt(ctx).JSON, t)
	require.NoError(t, err)

	r.CheckLocalItems(t, file1)
	r.CheckRemoteItems(t, fstest.NewItem("tictactoe/tictactoe/toe.txt", "hello world", t1))

	// result should not change second time, since src is unchanged
	ctx = predictDstFromLogger(ctx)
	err = Sync(ctx, r.Fremote, r.Flocal, true)
	testLoggerVsLsf(ctx, r.Fremote, operations.GetLoggerOpt(ctx).JSON, t)
	require.NoError(t, err)

	r.CheckLocalItems(t, file1)
	r.CheckRemoteItems(t, fstest.NewItem("tictactoe/tictactoe/toe.txt", "hello world", t1))
}

func TestSyntax(t *testing.T) {
	ctx := context.Background()
	transform.Opt = transform.Options{Flags: transform.Flags{NameTransform: []string{"prefix"}}}
	err := transform.Reload(ctx)
	assert.Error(t, err) // should error as required value is missing

	transform.Opt = transform.Options{Flags: transform.Flags{NameTransform: []string{"banana"}}}
	err = transform.Reload(ctx)
	assert.Error(t, err) // should error as unrecognized option

	transform.Opt = transform.Options{Flags: transform.Flags{NameTransform: []string{"=123"}}}
	err = transform.Reload(ctx)
	assert.Error(t, err) // should error as required key is missing

	transform.Opt = transform.Options{Flags: transform.Flags{NameTransform: []string{"prefix=123"}}}
	err = transform.Reload(ctx)
	assert.NoError(t, err) // should not error
}

func TestConflicting(t *testing.T) {
	ctx := context.Background()
	r := fstest.NewRun(t)
	transform.Opt = transform.Options{Flags: transform.Flags{NameTransform: []string{"prefix=tac", "trimprefix=tac"}}}
	err := transform.Reload(ctx)
	require.NoError(t, err)
	file1 := r.WriteFile("toe/toe/toe", "hello world", t1)

	r.Mkdir(ctx, r.Fremote)
	ctx = predictDstFromLogger(ctx)
	err = Sync(ctx, r.Fremote, r.Flocal, true)
	testLoggerVsLsf(ctx, r.Fremote, operations.GetLoggerOpt(ctx).JSON, t)
	require.NoError(t, err)

	// should result in no change as prefix and trimprefix cancel out
	r.CheckLocalItems(t, file1)
	r.CheckRemoteItems(t, fstest.NewItem("toe/toe/toe", "hello world", t1))
}
