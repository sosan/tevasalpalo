// Paquete runtimefs: extraccion de los assets embebidos (ZIP / tar.gz) al
// directorio runtime/ junto al ejecutable, con protecciones Zip/Tar Slip.

package runtimefs

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"tevasalpalo/internal/assets"
)

// ExtraerRuntime extrae el asset embebido (ZIP o tar.gz) en targetDir.
// El motor de Linux viene en tar.gz, así que ambos formatos conviven aquí.
func ExtraerRuntime(targetDir, pathFile string) error {
	f, err := openAsset(pathFile)
	if err != nil {
		return err
	}
	defer f.Close()

	if strings.HasSuffix(pathFile, ".tar.gz") {
		return extractTarGz(targetDir, f)
	}
	return extractZip(targetDir, f)
}

// openAsset abre un asset embebido del paquete assets (el runtime del motor
// y los bundles de Tor viven en el mismo embed.FS). extractRuntime lo usa con
// rutas tipo "assets/<fichero>".
func openAsset(pathFile string) (fs.File, error) {
	return assets.Dir.Open(pathFile)
}

// extractTarGz extrae un tar.gz embebido preservando el modo de los archivos
// (el motor de Linux necesita el bit +x en acestreamengine y los .so legibles).
func extractTarGz(targetDir string, f fs.File) error {
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("no se pudo leer el gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error leyendo el tar.gz: %w", err)
		}
		// No se aceptan rutas que salgan del directorio destino.
		dest, err := safeJoin(targetDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, hdr.FileInfo().Mode().Perm())
			if err != nil {
				return fmt.Errorf("no se pudo crear %s: %w", dest, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("error al copiar %s: %w", hdr.Name, err)
			}
			if err := out.Close(); err != nil {
				return fmt.Errorf("error al cerrar %s: %w", hdr.Name, err)
			}
		}
		// Symlinks y otros tipos se ignoran: el paquete no los trae.
	}
}

// safeJoin une base y name rechazando cualquier salto fuera de base
// (Zip Slip / Tar Slip).
func safeJoin(base, name string) (string, error) {
	clean := filepath.Clean(filepath.Join(base, name))
	baseAbs := filepath.Clean(base)
	if clean != baseAbs && !strings.HasPrefix(clean, baseAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("ruta fuera del directorio destino: %s", name)
	}
	return clean, nil
}

func extractZip(targetDir string, zipFile fs.File) error {
	zipInfo, _ := zipFile.Stat()
	zipSize := zipInfo.Size()

	zipReader, err := zip.NewReader(io.NewSectionReader(zipFile.(io.ReaderAt), 0, zipSize), zipSize)
	if err != nil {
		return fmt.Errorf("no se pudo leer el ZIP: %w", err)
	}

	// El motor son ~9000 ficheros: listar cada ruta inunda el log sin aportar
	// nada. Se cuenta y se resume al final.
	var nFiles, nDirs int

	for _, file := range zipReader.File {
		// safeJoin y no filepath.Join: una entrada "../evil.exe" en el ZIP
		// escribiría fuera del directorio destino (Zip Slip). extractTarGz ya lo
		// hace; el ZIP se quedó sin la protección.
		filePath, err := safeJoin(targetDir, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			nDirs++
			if err := os.MkdirAll(filePath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}

		inFile, err := file.Open()
		if err != nil {
			return fmt.Errorf("no se pudo abrir archivo en ZIP: %s: %v", file.Name, err)
		}
		outFile, err := os.Create(filePath)
		if err != nil {
			inFile.Close()
			return fmt.Errorf("no se pudo crear archivo: %s: %v", filePath, err)
		}

		_, err = io.Copy(outFile, inFile)
		inFile.Close()
		outFile.Close()
		if err != nil {
			return fmt.Errorf("error al copiar %s: %v", file.Name, err)
		}

		err = os.Chmod(filePath, file.Mode())
		if err != nil {
			return fmt.Errorf("error al cambiar permisos %s: %v", file.Name, err)
		}
		nFiles++
	}
	log.Printf("   %d archivos, %d carpetas", nFiles, nDirs)
	return nil
}

// ExisteFichero verifica si un archivo o directorio existe
func ExisteFichero(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
