package veyon

import (
	"crypto/rand"
	"fmt"
	"log"

	"github.com/DoimoJr/planck-proxy/internal/veyon/qds"
)

// SendFile invia un file sul PC studente usando la feature Veyon FileTransfer
// (UUID 4a70bd5a-...). Sequenza wire (da plugins/filetransfer/FileTransferController.cpp):
//
//  1. StartFileTransfer  → {TransferId, FileName, OverwriteExistingFile}
//  2. ContinueFileTransfer → {TransferId, DataChunk}  [ripetuto, chunk da 256 KB]
//  3. FinishFileTransfer  → {TransferId, FileName, OpenFileInApplication}
//
// Quando `openInApp == true`, il client studente apre il file con il
// programma associato (un .bat → cmd.exe lo esegue).
//
// Errore se la connessione si chiude a meta' invio, o se uno degli
// invii fallisce. Il TransferId e' un UUID v4 generato sul momento.
func (c *Conn) SendFile(filename string, content []byte, openInApp, overwrite bool) error {
	tid, err := newRandomUuid()
	if err != nil {
		return fmt.Errorf("veyon: generate transfer id: %w", err)
	}
	log.Printf("veyon: SendFile %s (%d byte) -> %s, tid=%s open=%v overwrite=%v",
		filename, len(content), c.cfg.Addr, tid.String(), openInApp, overwrite)

	// Le chiavi degli argomenti nei FeatureMessage Veyon sono gli INTEGER
	// dei membri dell'`enum class Argument` convertiti a stringa decimale.
	// Vedi `FeatureMessage::argument()` in core/src/FeatureMessage.h:
	//
	//     m_arguments[QString::number(static_cast<int>(index))]
	//
	// Per FileTransfer (plugins/filetransfer/FileTransferPlugin.h):
	//
	//     enum class Argument {
	//         TransferId,             // 0
	//         FileName,               // 1
	//         DataChunk,              // 2
	//         OpenFileInApplication,  // 3
	//         OverwriteExistingFile,  // 4
	//         Files,                  // 5
	//         CollectionId,           // 6
	//         FileSize,               // 7
	//     };

	// Step 1: StartFileTransfer.
	if err := c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureFileTransfer),
		Command:     CmdStartFileTransfer,
		Arguments: qds.VariantMap{
			"0": tid,       // TransferId
			"1": filename,  // FileName
			"4": overwrite, // OverwriteExistingFile
		},
	}); err != nil {
		return fmt.Errorf("veyon: StartFileTransfer: %w", err)
	}
	log.Printf("veyon: StartFileTransfer ok (tid=%s)", tid.String())

	// Step 2: ContinueFileTransfer (chunked).
	chunks := 0
	for offset := 0; offset < len(content); offset += FileTransferChunkSize {
		end := offset + FileTransferChunkSize
		if end > len(content) {
			end = len(content)
		}
		chunk := content[offset:end]
		if err := c.SendFeature(FeatureMessage{
			FeatureUUID: uuid(FeatureFileTransfer),
			Command:     CmdContinueFileTransfer,
			Arguments: qds.VariantMap{
				"0": tid,   // TransferId
				"2": chunk, // DataChunk
			},
		}); err != nil {
			return fmt.Errorf("veyon: ContinueFileTransfer @%d: %w", offset, err)
		}
		chunks++
	}
	log.Printf("veyon: ContinueFileTransfer ok (tid=%s, %d chunks)", tid.String(), chunks)

	// Step 3: FinishFileTransfer (con flag open-in-app).
	if err := c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureFileTransfer),
		Command:     CmdFinishFileTransfer,
		Arguments: qds.VariantMap{
			"0": tid,       // TransferId
			"1": filename,  // FileName
			"3": openInApp, // OpenFileInApplication
		},
	}); err != nil {
		return fmt.Errorf("veyon: FinishFileTransfer: %w", err)
	}
	log.Printf("veyon: FinishFileTransfer ok (tid=%s)", tid.String())
	return nil
}

// newRandomUuid produce 16 byte random (UUID v4 senza version bits set
// strict — Veyon usa il QUuid come opaque identifier, non lo valida).
func newRandomUuid() (qds.QUuid, error) {
	var u qds.QUuid
	_, err := rand.Read(u[:])
	if err != nil {
		return u, err
	}
	// Setta version=4 + variant=10 secondo RFC 4122, per pulizia
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80
	return u, nil
}
