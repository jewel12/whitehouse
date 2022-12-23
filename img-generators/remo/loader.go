package remo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/math/fixed"
	"google.golang.org/api/drive/v3"
)

type Appliance struct {
	SmartMeter SmartMeter `json:"smart_meter"`
}

type SmartMeter struct {
	EcoNetLiteProperties []*EcoNetLiteProperty `json:"echonetlite_properties"`
}

type EcoNetLiteProperty struct {
	Name string `json:"name"`
	Epc  int    `json:"epc"`
	Val  string `json:"val"`
}

const (
	MeasuredInstantaneous int = 231
)

func Load() error {
	measured, err := findMeasuredInstantaneous()
	if err != nil {
		return err
	}
	buf, err := drawMeasured(measured)
	if err != nil {
		return err
	}
	if err := putFile(buf, os.Getenv("IMG_DRIVE_ID"), "remo.jpeg"); err != nil {
		return err
	}
	return nil
}

// いろいろ置けるようにしておく
// putFile dstで指定されたGoogle Drive IDにfileを置く
// Driveには同名のファイルが存在できるため、すでにfileNameが存在する場合は削除して配置する
func putFile(file io.Reader, dst string, fileName string) error {
	ctx := context.Background()

	srv, err := drive.NewService(ctx)
	if err != nil {
		return fmt.Errorf("GoogleDriveサービス作成に失敗 %w", err)
	}

	r, err := srv.Files.List().PageSize(1000).Fields("files(id, name)").Q(fmt.Sprintf("'%s' in parents", dst)).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("ファイルの検索に失敗 %w", err)
	}
	for _, f := range r.Files {
		if f.Name == fileName {
			if err := srv.Files.Delete(f.Id).Do(); err != nil {
				return fmt.Errorf("ファイルの削除に失敗 %w", err)
			}
		}
	}

	remojpg := &drive.File{Name: fileName, Parents: []string{dst}}

	_, err = srv.Files.Create(remojpg).Media(file).Do()
	if err != nil {
		return fmt.Errorf("GoogleDriveへのファイル配置に失敗 %w", err)
	}

	return nil
}

// drawMeasured 瞬間電力計算値をjpegとして出力
// 画像サイズはM5Paper用に固定されている
func drawMeasured(measured float64) (*bytes.Buffer, error) {
	// M5Paperは横置きとする
	width := 960
	height := 540
	img := image.NewRGBA(image.Rectangle{Max: image.Point{X: width, Y: height}})
	ft, err := truetype.Parse(gobold.TTF)
	if err != nil {
		return nil, fmt.Errorf("フォントの読み込みに失敗 %w", err)
	}

	opt := truetype.Options{
		Size:              90,
		DPI:               0,
		Hinting:           0,
		GlyphCacheEntries: 0,
		SubPixelsX:        0,
		SubPixelsY:        0,
	}
	face := truetype.NewFace(ft, &opt)

	dr := &font.Drawer{
		Dst:  img,
		Src:  image.White,
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(100 * 64), Y: fixed.Int26_6(270 * 64)},
	}
	dr.DrawString(strconv.FormatFloat(measured, 'f', 2, 64) + " W")

	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, img, nil); err != nil {
		return nil, fmt.Errorf("jpegエンコードに失敗 %w", err)
	}
	return buf, nil
}

// findMeasuredInstantaneous 瞬時電力計算値を返す
// 瞬時電力計算値はNature Remo APIを叩いて取得している
func findMeasuredInstantaneous() (float64, error) {
	client := &http.Client{}

	req, err := http.NewRequest("GET", "https://api.nature.global/1/appliances", nil)
	if err != nil {
		// handle err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("REMO_API_TOKEN")))

	resp, err := client.Do(req)
	if err != nil {
		// handle err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// handle err
	}
	var appliances []*Appliance
	if err := json.Unmarshal(body, &appliances); err != nil {
		// handle err
	}
	return findMeasuredInstantaneousFromAppliances(appliances)
}

// findMeasuredInstantaneous NatureRemo APIレスポンスから瞬時電力計算値を取得して返す
// 完全にジュエル家専用のロジックとなっており、NatureRemo接続機器はひとつのみという制約がある。
// これらに合致しない場合はerrorが返る。
func findMeasuredInstantaneousFromAppliances(appliances []*Appliance) (float64, error) {
	if len(appliances) != 1 {
		return 0, fmt.Errorf("NatureRemo接続機器はひとつのみ")
	}
	for _, appliance := range appliances[0].SmartMeter.EcoNetLiteProperties {
		switch appliance.Epc {
		case MeasuredInstantaneous:
			f, err := strconv.ParseFloat(appliance.Val, 64)
			if err != nil {
				return 0, fmt.Errorf("floatへのパースに失敗 %w", err)
			}
			return f, nil
		}
	}
	return 0, fmt.Errorf("瞬時電力計測値が存在しない")
}
