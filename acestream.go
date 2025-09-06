package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
)

//go:embed assets/acestream-runtime-windows.zip
var runtimeZip embed.FS

const (
	runtimeDirName    = "runtime" // Carpeta junto al .exe
	httpPort          = 6878
	httpWebServerPort = 3000
	assetName         = "acestream-runtime-windows.zip"
)

// Opcionalmente, si los links tienen más información (como calidad, idioma, etc.)
type AcestreamLink struct {
	Hash     string `json:"hash"`
	Quality  string `json:"quality,omitempty"`  // Ej: "HD", "SD"
	Language string `json:"language,omitempty"` // Ej: "ES", "EN"
}

// Mapeo de broadcasters a su información detallada (incluyendo múltiples links)
var broadcasterToAcestream = map[string]BroadcasterInfo{
	"DAZN 1": {
		Links: []string{
			"b03f9310155cf3b4eafc1dfba763781abc3ff025",
			"36394be1db2e20b5997d987c32fd38c7f0f194b7",
			// "ee4dc45720d3ec283f61189fbfc120c91d5141bf",
			// "e2887cdef86768a4253e9810169943a07e54cf62",
			// "6538b79ce0da657d8455d1da6a5f342398899a0e",
			"50a8a13c474848d1efbd5586efdb5b6cdd173fa9",
			// "688e785893b50acc1d00cb37f15bfc42e72f5ae3",
			// "4141892f5df7d6474bf0279895ce02b7336c9928",
			// "0560234787945a17522e284c4c22bb4df29f33b0",
		},
	},
	"DAZN": {
		Links: []string{
			"b03f9310155cf3b4eafc1dfba763781abc3ff025",
			"36394be1db2e20b5997d987c32fd38c7f0f194b7",
			// "ee4dc45720d3ec283f61189fbfc120c91d5141bf",
			// "e2887cdef86768a4253e9810169943a07e54cf62",
			// "6538b79ce0da657d8455d1da6a5f342398899a0e",
			"50a8a13c474848d1efbd5586efdb5b6cdd173fa9",
			"688e785893b50acc1d00cb37f15bfc42e72f5ae3",
			"4141892f5df7d6474bf0279895ce02b7336c9928",
			"0560234787945a17522e284c4c22bb4df29f33b0",
		},
	},
	"DAZN 2": {
		Links: []string{
			"43004e955731cd3afcc34d24e5178d4b427bff37",
			"b0eabe0fdd02fdd165896236765a9b753a2ff516",
			// "9adf7ac6531788ec022dbc14b77e1367f6c5bdc5",
		},
	},
	"DAZN 3": {
		Links: []string{
			// "6fb944e8985881ae6db89667aca6362e746255b6",
		},
	},
	"DAZN 4": {
		Links: []string{
			"4e401fdceebffdf1f09aef954844d09f6c62f460",
			"eb884f77ce8815cf1028c4d85e8b092c27ea1693",
			"6a11eb510edc5b3581c5c97883c44563eb894b1b",
			"7e90956539f4e1318a63f3960e4f75c7c7c5a3b8",
			"c21a2524a8de3e1e5b126f2677a3e993d9aa07c4",
		},
	},
	"DAZN LALIGA": {
		Links: []string{
			"0e50439e68aa2435b38f0563bb2f2e98f32ff4b1",
			"1bb5bf76fb2018d6db9aaa29b1467ecdfabe2884",
			"f8d5e39a49b9da0215bbd3d9efb8fb3d06b76892",
		},
	},
	"DAZN LALIGA 2": {
		Links: []string{
			"5091ea94b75ba4b50b078b4102a3d0e158ef59c3",
			"c976c7b37964322752db562b4ad65515509c8d36",
		},
	},
	"DAZN EVENTOS": {
		Links: []string{
			"176f0b8565a161f87c81a4a250463e3eaf678fb5",
			// "8bdeb6055da0be3bd1e1977dbf3640408f7d0267",
			// "4b048bcfaed7daec454e88f0e29b56756300447d",
		},
	},
	"DSPORT": {
		Links: []string{
			"8bdeb6055da0be3bd1e1977dbf3640408f7d0267",
		},
	},
	"ESPN DEPORTES": {
		Links: []string{
			"4b048bcfaed7daec454e88f0e29b56756300447d",
		},
	},
	"TNT PREMIUM": {
		Links: []string{
			"8bdeb6055da0be3bd1e1977dbf3640408f7d0267",
		},
	},
	"DAZN F1": {
		Links: []string{
			"38e9ae1ee0c96d7c6187c9c4cc60ffccb565bdf7",
			"3f5b7e2f883fe4b4b973e198d147a513d5c7c32a",
			"ba6e1bdc8e03da60ff572557645eb04370af0cd0",
			"8c1155cdfae76eb582290c04904c98da066657c9",
			"b08e158ea3f5c72084f5ff8e3c30ca2e4d1ff6d1",
			// "bcf9dc38f92e90a71b87bd54b3bac91b76d09a69",
			"fd53cfa7055fe458d4f5c0ff59a06cd43723be55",
			"ed6188dcbb491efeb2092c1a6199226c27f61727",
			// "d27584ebe5128c8033cb6fdc806a994fbd17b790",
			"6422e8bc34282871634c81947be093c04ad1bb29",
			// "c9c18ae7a9dafba1caae5beb22060f9c92bba553",
		},
	},
	"DAZN LALIGA TV": {
		Links: []string{
			"0e50439e68aa2435b38f0563bb2f2e98f32ff4b1",
			"1bb5bf76fb2018d6db9aaa29b1467ecdfabe2884",
			"f8d5e39a49b9da0215bbd3d9efb8fb3d06b76892",
			"520950d296c952e1864a08e15af9f89f1ab514ec",
			// "e2b8a4aba2f4ea3dd68992fcdb65c9e62d910b05",
			// "4e6d9cf7d177366045d33cd8311d8b1d7f4bed1f",
		},
	},
	"M+ LALIGA": {
		Links: []string{
			"107c3ce5a5d2527c9f06e4eab87477201791f231",
			"d2ddf9802ccfdc456f872eea4d24596880a638a0",
			"14b6cd8769cd485f2cffdca64be9698d9bfeac58",
			"07f2b92762cfff99bba785c2f5260c7934ca6034",
			"4b528d10eaad747ddf52251206177573ee3e9f74",
			"d3de78aebe544611a2347f54d5796bd87f16c92d",
		},
	},
	"M+ LALIGA TV": {
		Links: []string{
			"14b6cd8769cd485f2cffdca64be9698d9bfeac58",
			"07f2b92762cfff99bba785c2f5260c7934ca6034",
			"4b528d10eaad747ddf52251206177573ee3e9f74",
			"d3de78aebe544611a2347f54d5796bd87f16c92d",
		},
	},
	"M+ LALIGA 2": {
		Links: []string{
			"911ad127726234b97658498a8b790fdd7516541d",
			"51b363b1c4d42724e05240ad068ad219df8042ec",
			"ad42faa399df66dcd62a1cbc9d1c99ed4512d3b8",
		},
	},
	"M+ LALIGA 2 TV": {
		Links: []string{
			"911ad127726234b97658498a8b790fdd7516541d",
			"51b363b1c4d42724e05240ad068ad219df8042ec",
			"ad42faa399df66dcd62a1cbc9d1c99ed4512d3b8",
		},
	},
	"M+ LALIGA 3": {
		Links: []string{
			"382b14499e3d76e557d449d2e5bbc4e4bd63bd39",
		},
	},
	"M+ LIGA DE CAMPEONES": {
		Links: []string{
			"0f7842f8b6c26571e5a974407b61623e56e6a052",
			"f3eea003e23f94dc2d527306de9dd386e3ebf4ba",
			"680187938f9305cce3ae240298f10e8695bf77c2",
			// "8c1c3eae077f3a786ed2f0a426197ea93fdf7373",
			"e572a5178ff72eed7d1d751a18b4b3419699f370",
			"c16b4fab1f724c94cad92081cbb7fc7c6fe8a2cc",
			"afbf2a479c5a5072698139f0f556ef3e77a99bd0",
			"dfa66881b9613a77bd5479f6eedc5542504c3ef7",
			// "97df5b7824948972d041d8ca2a4d29c90b641bc9",
			// "8c1c3eae077f3a786ed2f0a426197ea93fdf7373",
			// "dfa66881b9613a77bd5479f6eedc5542504c3ef7",
			// "e572a5178ff72eed7d1d751a18b4b3419699f370",
			// "2b51710cee513e8939785fa3e7980f32d4e0415f",
			// "9db029dff6a9c637d1f670e78dbc1a479b9b406e",
			// "b028202ff335911db3118bceac027df3e8ef6c32",
		},
	},
	"M+ LIGA DE CAMPEONES 2": {
		Links: []string{
			"e7d8cae7f693fe46e89bbf74820caac9ffb32a30",
			"33c009a025508cb2186b9ce36279640bb2507bdf",
			"74ab4e4ec7e2da001f473ca40893b7307b8029c5",
			"4fc6d0331830ad8743abab2fe2473b63bdfbc49f",

			// "38f7b2044e549df2039ff26cefa6f9a60c854d5e",
		},
	},
	"M+ LIGA DE CAMPEONES 3": {
		Links: []string{
			"2b5129adc57d43790634d796fe3468b9fd061259",
			"17b8bc4bf8e29547b0071c742e3d7da3bcbc484d",
			"4416843c96b7f7a1bc55c476091a60fff0922bc7",
			// "cfc371890bfb502737a26de5215e50929c52d0f9",
		},
	},
	"M+ LIGA DE CAMPEONES 4": {
		Links: []string{
			"77998f8161373611ff6b348e7eda5b4e97d3ec29",
		},
	},

	"M+ DEPORTES": {
		Links: []string{
			"5d3f582738467aaf213e408601aca5cc13fa9359",
			"3692ea4cdda97eb0062ed5d656ebd61f149ebd0b",
			"751b9cb03d188ce70e6aac22c1bfb16a5d0b2237",
			"ef9dcc4eaac36a0f608b52a31f8ab237859e071a",
			// "acb510858f34c3c6fd5f79395b031abd6885c2b3",
			"ebca4a63ce3bfda7b272964a1acc5227218184a4",
			"2f3cfd199a49819cbd129689a840dc3d23ab93aa",
		},
	},
	"M+ DEPORTES 2": {
		Links: []string{
			"73d38feaa770d788848ec098470e32670fe55a61",
			"438991226c3bc6a06e7bfda9bea9f769957dc366",
			"f0ee7a2b43c1df5ea9e4fac5bf876d5bef4372b0",
			"edd06f11e1cef292a1d795e15207ef2f580e298c",
			"bfa01c11c5c6b7a616a516de4f2c769a89d26b25",
		},
	},
	"M+ DEPORTES 3": {
		Links: []string{
			"29d786d72d4b53dbc333af3a50f8fb021aa0296f",
			"d5271a967767f761c8812c4b6195dd40f20126f7",
			"753d4b1f7c4ef43238b5cf23b05572b550a44eee",
			// "799c6b5ee1cf41af077d14e3f9c45a32697eb903",
			// "2fd410c5d89e7a627cd3785685b7915b8e4bd534",
		},
	},
	"M+ DEPORTES 4": {
		Links: []string{
			"37825883ed185365e3194a79207347f6c7bd5ba5",
			"ebf3f251c1e119aefc6a1efc95c9b5d1909249e2",
			"58a4c880ab18486d914751db32a12760e74b75a4",
			// "b40e1de2dcbd7c665f54877b14c830ed67b32a96",
			// "7b361369a40046ad3011086f9d4ae2982fb4d5aa",
		},
	},
	"M+ DEPORTES 5": {
		Links: []string{
			"6dc4ac4eeae18e9daec433b01db82435cf84c57c",
			"9b84af74b2fa3690c519199326fc2f181b025cdd",
			"0b708083541a46dc15216c8003d7bcf39c458b2a",
		},
	},
	"M+ DEPORTES 6": {
		Links: []string{
			"190a81938c2ddc6fe97758271f8c48f4db31c28a",
		},
	},

	"M+ VAMOS": {
		Links: []string{
			"4e99e755aa32c4bc043a4bb1cd1de35f9bd94dde",
			"1444a976d2cf6e7fdcee833ed36ee5e55632253f",
			"c7c81acdd1a03ecc418c94c2f28e2adb0556c40b",
			"3b2a8b41e7097c16b0468b42d7de0320886fa933",
			"2940120bf034db79a7f5849846ccea0255172eae",
		},
	},

	"M+ GOLF": {
		Links: []string{
			"76a69812c66bfc4899e89df498220588a56e6064",
			"872608e734992db636eb79426802cd08f4029afb",
		},
	},
	"Movistar Golf": {
		Links: []string{
			"76a69812c66bfc4899e89df498220588a56e6064",
			"872608e734992db636eb79426802cd08f4029afb",
		},
	},
	"M+": {
		Links: []string{
			"199190e3f28c1de0be34bccf0d3568dc65209b99",
			"5866e83279307bf919068ae7a0d250e4e424e464",
			"5d9a26e0a5f3e5f2ae027bd05635ab9a4fd4b51a",
			"5e24a1b9187fccb91553f7c7da4b36341386f74a",
			"1ab443f5b4beb6d586f19e8b25b9f9646cf2ab78",
		},
	},
	"Movistar Plus+": {
		Links: []string{
			"199190e3f28c1de0be34bccf0d3568dc65209b99",
			"5866e83279307bf919068ae7a0d250e4e424e464",
			"5d9a26e0a5f3e5f2ae027bd05635ab9a4fd4b51a",
			"5e24a1b9187fccb91553f7c7da4b36341386f74a",
		},
	},
	"Movistar Plus+ 2": {
		Links: []string{},
	},
	"Movistar Plus": {
		Links: []string{
			"199190e3f28c1de0be34bccf0d3568dc65209b99",
			"5866e83279307bf919068ae7a0d250e4e424e464",
			"5d9a26e0a5f3e5f2ae027bd05635ab9a4fd4b51a",
			"5e24a1b9187fccb91553f7c7da4b36341386f74a",
		},
	},
	"LALIGA HYPERMOTION": {
		Links: []string{
			"8ee52f6208e33706171856f99d2ed2dabd317f3a",
			"70f22be1286ef224b5e4e9451d9a42468152cda4",
			"f15f997f457e49ad9697e65cf2d78db26ee875b9",
			"ff38b875b60074d60edb64cf10d09b32370a7135",
			"778d2f60bb7207addedcca0b9aed98f41529724e",
		},
	},
	"LALIGA HYPERMOTION 2": {
		Links: []string{
			"8a05571c0c8fe53f160fb7d116cdf97243679e86",
		},
	},
	"LALIGA HYPERMOTION 3": {
		Links: []string{
			"1ba18731a8e18bb4b3a5dfa5bb7b0f05762849a6",
		},
	},
	"LALIGA TV HYPERMOTION": {
		Links: []string{
			"8ee52f6208e33706171856f99d2ed2dabd317f3a",
			"70f22be1286ef224b5e4e9451d9a42468152cda4",
			"f15f997f457e49ad9697e65cf2d78db26ee875b9",
			"ff38b875b60074d60edb64cf10d09b32370a7135",
			"778d2f60bb7207addedcca0b9aed98f41529724e",
		},
	},

	"GOL": {
		Links: []string{
			"b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
			"472d1f3a658a51bcab0ffa9138e1e28a05fba30b",
			"b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
		},
	},
	"GOL TV": {
		Links: []string{
			"b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
			"472d1f3a658a51bcab0ffa9138e1e28a05fba30b",
			"b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
		},
	},
	"GOLTV PLAY": {
		Links: []string{
			"b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
			"472d1f3a658a51bcab0ffa9138e1e28a05fba30b",
			"b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
		},
	},
	"EUROSPORT 1": {
		Links: []string{
			"ef2abf419586d9876370be127ad592dbb41b126a",
			"bae98f69fbf867550b4f73b4eb176dae84d7f909",
			"714e14e6d6e27660fd25a75b57b0ac5580fe705d",
			// "c3da6c4f91d9d10ade00318a869435e19f204d0e",
		},
	},
	"EUROSPORT 2": {
		Links: []string{
			"37d6f1aabcde81ee6e4873b4db6b7bb8964af8bf",
			"dc4ccb9e72550bc72be9360aa7d77e337ad11ecc",
			"0585e09bb8ac9720e4c11934f1b184e309291551",
			"5c910d614894635153a7d42de98cc2e4a958a53f",
		},
	},
	"M+ ELLAS VAMOS": {
		Links: []string{
			"9b84af74b2fa3690c519199326fc2f181b025cdd",
			// "c7c81acdd1a03ecc418c94c2f28e2adb0556c40b",
			// "3b2a8b41e7097c16b0468b42d7de0320886fa933",
			// "2940120bf034db79a7f5849846ccea0255172eae",
		},
	},
	"LALIGA TV BAR": {
		Links: []string{
			"608b0faf7d3d25f6fe5dba13d5e4b4142949990e",
		},
	},
}

// findLinkForBroadcaster busca un enlace para un nombre de broadcaster.
// Prioriza la coincidencia exacta, luego parcial.
func findLinkForBroadcaster(name string) []string {
	// Coincidencia exacta
	// quizas pasarlo a minisculas
	nameUpper := strings.ToUpper(name)
	if dataAce, exists := broadcasterToAcestream[nameUpper]; exists {
		return dataAce.Links
	}

	// Coincidencia parcial (como antes)
	// nameUpper := strings.ToUpper(name)
	for key, dataAce := range broadcasterToAcestream {
		baseKey := strings.Split(key, " [")[0]
		if strings.Contains(nameUpper, strings.ToUpper(baseKey)) {
			// Preferir coincidencia exacta de base si es posible
			if nameUpper == strings.ToUpper(baseKey) {
				return dataAce.Links
			}
			// Si no hay exacta, esta es una candidata (la última encontrada)
			// Para hacerlo más robusto, podrías tener lógica para elegir la mejor parcial
		}
	}
	// Si no se encontró parcial, devolver vacío
	return []string{}
}

func runAceStream() error {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal("No se pudo obtener la ruta del ejecutable: ", err)
	}
	execDir := filepath.Dir(exePath)
	runtimePath := filepath.Join(execDir, runtimeDirName)
	enginePath := filepath.Join(runtimePath, "engine", "ace_console.exe")

	if !fileExists(enginePath) {
		log.Println("📦 No se encontró Lista Canales TV. Extrayendo por primera vez...")
		if err := extractRuntime(runtimePath); err != nil {
			log.Fatal("Error al extraer Lista Canales: ", err)
		}
		log.Println("✅ Lista Canales TV extraído exitosamente.")
	} else {
		log.Println("🔁 Lista Canales TV ya existe. Usando versión existente.")
	}

	log.Println("🚀 Actualizando Lista Canales TV...")
	args := []string{
		"--live-buffer", "30",
		"--vod-buffer", "30",
		"--client-console",
	}
	cmd := exec.Command(enginePath, args...)
	cmd.Dir = filepath.Join(runtimePath, "engine")

	if err := cmd.Start(); err != nil {
		log.Fatal("No se pudo iniciar: ", err)
	}

	log.Println("⏳ Esperando a que termine de actualizarse la Lista Canales TV...")
	if !waitForAPI(fmt.Sprintf("http://localhost:%d/webui/api/service?method=get_version", httpPort), 30*time.Second) {
		log.Fatal("❌ No respondió después de 30 segundos")
	}

	log.Println("🎉 Lista Canales TV lista. Abriendo interfaz...")
	openBrowser(fmt.Sprintf("http://localhost:%d", httpWebServerPort))

	log.Println("✅ Todo listo. ¡A relajarse y disfrutar del contenido! 🍿")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("🛑 Cerrando...")
	if cmd != nil && cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}

// extractRuntime extrae el ZIP embebido en el directorio runtime
func extractRuntime(targetDir string) error {
	// Abrir el ZIP embebido
	zipFile, err := runtimeZip.Open("assets/acestream-runtime-windows.zip")
	if err != nil {
		return fmt.Errorf("no se pudo abrir el ZIP embebido: %w", err)
	}
	defer zipFile.Close()

	// Obtener tamaño
	zipInfo, _ := zipFile.Stat()
	zipSize := zipInfo.Size()

	// Crear reader ZIP
	zipReader, err := zip.NewReader(io.NewSectionReader(zipFile.(io.ReaderAt), 0, zipSize), zipSize)
	if err != nil {
		return fmt.Errorf("no se pudo leer el ZIP: %w", err)
	}

	// Extraer cada archivo
	for _, file := range zipReader.File {
		filePath := filepath.Join(targetDir, file.Name)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, 0755); err != nil {
				return err
			}
			continue
		}

		// Crear directorios intermedios
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}

		// Extraer archivo
		inFile, err := file.Open()
		if err != nil {
			return fmt.Errorf("no se pudo abrir archivo en ZIP: %s: %w", file.Name, err)
		}
		outFile, err := os.Create(filePath)
		if err != nil {
			inFile.Close()
			return fmt.Errorf("no se pudo crear archivo: %s: %w", filePath, err)
		}

		_, err = io.Copy(outFile, inFile)
		inFile.Close()
		outFile.Close()
		if err != nil {
			return fmt.Errorf("error al copiar %s: %w", file.Name, err)
		}

		// Aplicar permisos (opcional en Windows)
		os.Chmod(filePath, file.Mode())
	}
	return nil
}

// fileExists verifica si un archivo o directorio existe
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// waitForAPI espera a que la API responda con 200 OK
func waitForAPI(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return true
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// openBrowser abre el navegador según el sistema
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		case "darwin":
			cmd = exec.Command("open", url)
		default: // Linux, etc.
			cmd = exec.Command("xdg-open", url)
		}
	}
	_ = cmd.Start()
}

func fetchUpdatedList() error {
	body, err := FetchWebData("https://shickat.me/")
	if err != nil {
		return err
	}
	extractedData := extractDataFromWebShitkat(body)
	updateBroadcasterMapWithGateway(broadcasterToAcestream, extractedData)

	return err
}

func extractDataFromWebShitkat(body []byte) map[string][]string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	extractedData := make(map[string][]string)
	doc.Find(".canal-card").Each(func(i int, card *goquery.Selection) {
		nombre := card.Find(".canal-nombre").Text()
		acestreamLink := strings.TrimSpace(card.Find(".acestream-link").Text()) //.AttrOr("href", "")
		extractedData[nombre] = append(extractedData[nombre], acestreamLink)
	})
	return extractedData
}
