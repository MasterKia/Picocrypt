package main

/*

Picocrypt v1.29
Copyright (c) Evan Su (https://evansu.cc)
Released under a GNU GPL v3 License
https://github.com/HACKERALERT/Picocrypt

~ In cryptography we trust ~

*/

import (
	"archive/zip"
	"bytes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"hash"
	"image"
	"image/color"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/HACKERALERT/clipboard"
	"github.com/HACKERALERT/crypto/argon2"
	"github.com/HACKERALERT/crypto/blake2b"
	"github.com/HACKERALERT/crypto/chacha20"
	"github.com/HACKERALERT/crypto/hkdf"
	"github.com/HACKERALERT/crypto/sha3"
	"github.com/HACKERALERT/dialog"
	"github.com/HACKERALERT/giu"
	"github.com/HACKERALERT/imgui-go"
	"github.com/HACKERALERT/infectious"
	"github.com/HACKERALERT/serpent"
	"github.com/HACKERALERT/zxcvbn-go"
)

// Constants
var KiB = 1 << 10
var MiB = 1 << 20
var GiB = 1 << 30
var TiB = 1 << 40
var WHITE = color.RGBA{0xff, 0xff, 0xff, 0xff}
var RED = color.RGBA{0xff, 0x00, 0x00, 0xff}
var GREEN = color.RGBA{0x00, 0xff, 0x00, 0xff}
var YELLOW = color.RGBA{0xff, 0xff, 0x00, 0xff}
var TRANSPARENT = color.RGBA{0x00, 0x00, 0x00, 0x00}

// Generic variables
var window *giu.MasterWindow
var version = "v1.29"
var dpi float32
var mode string
var working bool
var scanning bool

// Popup modals
var modalId int
var showPassgen bool
var showKeyfile bool
var showOverwrite bool
var showProgress bool

// Input and output files
var inputFile string
var inputFileOld string
var outputFile string
var onlyFiles []string
var onlyFolders []string
var allFiles []string
var inputLabel = "Drop files and folders into this window."

// Password and confirm password
var password string
var cpassword string
var passwordStrength int
var passwordState = giu.InputTextFlagsPassword
var passwordStateLabel = "Show"

// Password generator
var passgenLength int32 = 32
var passgenUpper bool
var passgenLower bool
var passgenNums bool
var passgenSymbols bool
var passgenCopy bool

// Keyfile variables
var keyfile bool
var keyfiles []string
var keyfileOrdered bool
var keyfileLabel = "None selected."

// Comments variables
var comments string
var commentsLabel = "Comments:"
var commentsDisabled bool

// Advanced options
var paranoid bool
var reedsolo bool
var split bool
var splitSize string
var splitUnits = []string{"KiB", "MiB", "GiB", "TiB", "Total"}
var splitSelected int32 = 1
var recombine bool
var compress bool
var delete bool
var keep bool
var kept bool

// Status variables
var startLabel = "Start"
var mainStatus = "Ready."
var mainStatusColor = WHITE
var popupStatus string

// Progress variables
var progress float32
var progressInfo string
var speed float64
var eta string
var canCancel bool

// Reed-Solomon encoders
var rs1, _ = infectious.NewFEC(1, 3)
var rs5, _ = infectious.NewFEC(5, 15)
var rs16, _ = infectious.NewFEC(16, 48)
var rs24, _ = infectious.NewFEC(24, 72)
var rs32, _ = infectious.NewFEC(32, 96)
var rs64, _ = infectious.NewFEC(64, 192)
var rs128, _ = infectious.NewFEC(128, 136)
var fastDecode bool

// Compression variables and passthrough
var compressDone int64
var compressTotal int64
var compressStart time.Time

type compressorProgress struct {
	io.Reader
}

func (p *compressorProgress) Read(data []byte) (int, error) {
	if !working {
		return 0, io.EOF
	}
	read, err := p.Reader.Read(data)
	compressDone += int64(read)
	progress, speed, eta = statify(compressDone, compressTotal, compressStart)
	if compress {
		popupStatus = fmt.Sprintf("Compressing at %.2f MiB/s (ETA: %s)", speed, eta)
	} else {
		popupStatus = fmt.Sprintf("Combining at %.2f MiB/s (ETA: %s)", speed, eta)
	}
	giu.Update()
	return read, err
}

// The main user interface
func draw() {
	giu.SingleWindow().Flags(524351).Layout(
		giu.Custom(func() {
			if showPassgen {
				giu.PopupModal("Generate password:##"+strconv.Itoa(modalId)).Flags(6).Layout(
					giu.Row(
						giu.Label("Length:"),
						giu.SliderInt(&passgenLength, 4, 64).Size(giu.Auto),
					),
					giu.Checkbox("Uppercase", &passgenUpper),
					giu.Checkbox("Lowercase", &passgenLower),
					giu.Checkbox("Numbers", &passgenNums),
					giu.Checkbox("Symbols", &passgenSymbols),
					giu.Checkbox("Copy to clipboard", &passgenCopy),
					giu.Row(
						giu.Button("Cancel").Size(100, 0).OnClick(func() {
							giu.CloseCurrentPopup()
							showPassgen = false
						}),
						giu.Style().SetDisabled(!(passgenUpper || passgenLower || passgenNums || passgenSymbols)).To(
							giu.Button("Generate").Size(100, 0).OnClick(func() {
								password = genPassword()
								cpassword = password
								passwordStrength = zxcvbn.PasswordStrength(password, nil).Score

								giu.CloseCurrentPopup()
								showPassgen = false
							}),
						),
					),
				).Build()
				giu.OpenPopup("Generate password:##" + strconv.Itoa(modalId))
				giu.Update()
			}

			if showKeyfile {
				giu.PopupModal("Manage keyfiles:##"+strconv.Itoa(modalId)).Flags(70).Layout(
					giu.Label("Drag and drop your keyfiles here."),
					giu.Custom(func() {
						if mode != "decrypt" {
							giu.Checkbox("Require correct order", &keyfileOrdered).Build()
							giu.Tooltip("Decryption will require the correct keyfile order.").Build()
						} else if keyfileOrdered {
							giu.Label("Correct order is required.").Build()
						}
					}),
					giu.Separator(),
					giu.Custom(func() {
						for _, i := range keyfiles {
							giu.Label(filepath.Base(i)).Build()
						}
					}),
					giu.Row(
						giu.Button("Clear").Size(100, 0).OnClick(func() {
							keyfiles = nil
							if keyfile {
								keyfileLabel = "Keyfiles required."
							} else {
								keyfileLabel = "None selected."
							}
							modalId++
							giu.Update()
						}),
						giu.Tooltip("Remove all keyfiles."),

						giu.Button("Done").Size(100, 0).OnClick(func() {
							giu.CloseCurrentPopup()
							showKeyfile = false
						}),
					),
				).Build()
				giu.OpenPopup("Manage keyfiles:##" + strconv.Itoa(modalId))
				giu.Update()
			}

			if showOverwrite {
				giu.PopupModal("Warning:##"+strconv.Itoa(modalId)).Flags(6).Layout(
					giu.Label("Output already exists. Overwrite?"),
					giu.Row(
						giu.Button("No").Size(100, 0).OnClick(func() {
							giu.CloseCurrentPopup()
							showOverwrite = false
						}),
						giu.Button("Yes").Size(100, 0).OnClick(func() {
							giu.CloseCurrentPopup()
							showOverwrite = false

							showProgress = true
							fastDecode = true
							canCancel = true
							modalId++
							giu.Update()
							go func() {
								work()
								working = false
								showProgress = false
								giu.Update()
							}()
						}),
					),
				).Build()
				giu.OpenPopup("Warning:##" + strconv.Itoa(modalId))
				giu.Update()
			}

			if showProgress {
				giu.PopupModal(" ##"+strconv.Itoa(modalId)).Flags(6).Layout(
					giu.Row(
						giu.ProgressBar(progress).Size(210, 0).Overlay(progressInfo),
						giu.Style().SetDisabled(!canCancel).To(
							giu.Button(func() string {
								if working {
									return "Cancel"
								}
								return "..."
							}()).Size(58, 0).OnClick(func() {
								working = false
								canCancel = false
							}),
						),
					),
					giu.Label(popupStatus),
				).Build()
				giu.OpenPopup(" ##" + strconv.Itoa(modalId))
				giu.Update()
			}
		}),

		giu.Row(
			giu.Label(inputLabel),
			giu.Custom(func() {
				bw, _ := giu.CalcTextSize("Clear")
				p, _ := giu.GetWindowPadding()
				bw += p * 2
				giu.Dummy((bw+p)/-dpi, 0).Build()
				giu.SameLine()
				giu.Style().SetDisabled((len(allFiles) == 0 && len(onlyFiles) == 0) || scanning).To(
					giu.Button("Clear").Size(bw/dpi, 0).OnClick(resetUI),
					giu.Tooltip("Clear all input files and reset UI state."),
				).Build()
			}),
		),

		giu.Separator(),
		giu.Style().SetDisabled((len(allFiles) == 0 && len(onlyFiles) == 0) || scanning).To(
			giu.Label("Password:"),
			giu.Row(
				giu.Button(passwordStateLabel).Size(54, 0).OnClick(func() {
					if passwordState == giu.InputTextFlagsPassword {
						passwordState = giu.InputTextFlagsNone
						passwordStateLabel = "Hide"
					} else {
						passwordState = giu.InputTextFlagsPassword
						passwordStateLabel = "Show"
					}
					giu.Update()
				}),
				giu.Tooltip("Toggle the visibility of password entries."),

				giu.Button("Clear").Size(54, 0).OnClick(func() {
					password = ""
					cpassword = ""
					giu.Update()
				}),
				giu.Tooltip("Clear the password entries."),

				giu.Button("Copy").Size(54, 0).OnClick(func() {
					clipboard.WriteAll(password)
					giu.Update()
				}),
				giu.Tooltip("Copy the password into your clipboard."),

				giu.Button("Paste").Size(54, 0).OnClick(func() {
					tmp, _ := clipboard.ReadAll()
					password = tmp
					if mode != "decrypt" {
						cpassword = tmp
					}
					passwordStrength = zxcvbn.PasswordStrength(password, nil).Score
					giu.Update()
				}),
				giu.Tooltip("Paste a password from your clipboard."),

				giu.Style().SetDisabled(mode == "decrypt").To(
					giu.Button("Create").Size(54, 0).OnClick(func() {
						showPassgen = true
						modalId++
						giu.Update()
					}),
				),
				giu.Tooltip("Generate a cryptographically secure password."),
			),
			giu.Row(
				giu.InputText(&password).Flags(passwordState).Size(302/dpi).OnChange(func() {
					passwordStrength = zxcvbn.PasswordStrength(password, nil).Score
					giu.Update()
				}),
				giu.Custom(func() {
					c := giu.GetCanvas()
					p := giu.GetCursorScreenPos()
					col := color.RGBA{
						uint8(0xc8 - 31*passwordStrength),
						uint8(0x4c + 31*passwordStrength), 0x4b, 0xff,
					}
					if password == "" || mode == "decrypt" {
						col = TRANSPARENT
					}
					path := p.Add(image.Pt(
						int(math.Round(-20*float64(dpi))),
						int(math.Round(12*float64(dpi))),
					))
					c.PathArcTo(path, 6*dpi, -math.Pi/2, math.Pi*(.4*float32(passwordStrength)-.1), -1)
					c.PathStroke(col, false, 2)
				}),
			),

			giu.Dummy(0, 0),
			giu.Style().SetDisabled(password == "" || mode == "decrypt").To(
				giu.Label("Confirm password:"),
				giu.Row(
					giu.InputText(&cpassword).Flags(passwordState).Size(302/dpi),
					giu.Custom(func() {
						c := giu.GetCanvas()
						p := giu.GetCursorScreenPos()
						col := color.RGBA{0x4c, 0xc8, 0x4b, 0xff}
						if cpassword != password {
							col = color.RGBA{0xc8, 0x4c, 0x4b, 0xff}
						}
						if password == "" || cpassword == "" || mode == "decrypt" {
							col = TRANSPARENT
						}
						path := p.Add(image.Pt(
							int(math.Round(-20*float64(dpi))),
							int(math.Round(12*float64(dpi))),
						))
						c.PathArcTo(path, 6*dpi, 0, 2*math.Pi, -1)
						c.PathStroke(col, false, 2)
					}),
				),
			),

			giu.Dummy(0, 0),
			giu.Style().SetDisabled(mode == "decrypt" && !keyfile).To(
				giu.Row(
					giu.Label("Keyfiles:"),
					giu.Button("Edit").Size(54, 0).OnClick(func() {
						showKeyfile = true
						modalId++
						giu.Update()
					}),
					giu.Tooltip("Manage your keyfiles."),

					giu.Style().SetDisabled(mode == "decrypt").To(
						giu.Button("Create").Size(54, 0).OnClick(func() {
							f := dialog.File().Title("Choose where to save the keyfile.")
							f.SetStartDir(func() string {
								if len(onlyFiles) > 0 {
									return filepath.Dir(onlyFiles[0])
								}
								return filepath.Dir(onlyFolders[0])
							}())
							f.SetInitFilename("Keyfile")
							file, err := f.Save()
							if file == "" || err != nil {
								return
							}

							fout, _ := os.Create(file)
							data := make([]byte, MiB)
							rand.Read(data)
							fout.Write(data)
							fout.Close()
						}),
						giu.Tooltip("Generate a cryptographically secure keyfile."),
					),
					giu.Style().SetDisabled(true).To(
						giu.InputText(&keyfileLabel).Size(giu.Auto),
					),
				),
			),
		),

		giu.Separator(),
		giu.Style().SetDisabled(mode != "decrypt" && ((len(keyfiles) == 0 && password == "") || (password != cpassword))).To(
			giu.Style().SetDisabled(mode == "decrypt" && (comments == "" || comments == "Comments are corrupted.")).To(
				giu.Label(commentsLabel),
				giu.InputText(&comments).Size(giu.Auto).Flags(func() giu.InputTextFlags {
					if commentsDisabled {
						return giu.InputTextFlagsReadOnly
					}
					return giu.InputTextFlagsNone
				}()),
			),
		),
		giu.Style().SetDisabled((len(keyfiles) == 0 && password == "") || (mode == "encrypt" && password != cpassword)).To(
			giu.Label("Advanced:"),
			giu.Custom(func() {
				if mode != "decrypt" {
					giu.Row(
						giu.Checkbox("Paranoid mode", &paranoid),
						giu.Tooltip("Provides the highest level of security attainable."),
						giu.Dummy(-170, 0),
						giu.Checkbox("Compress files", &compress).OnChange(func() {
							if !(len(allFiles) > 1 || len(onlyFolders) > 0) {
								if compress {
									outputFile = filepath.Join(filepath.Dir(outputFile), "Encrypted") + ".zip.pcv"
								} else {
									outputFile = filepath.Join(filepath.Dir(outputFile), filepath.Base(inputFile)) + ".pcv"
								}
							}
						}),
						giu.Tooltip("Compress files with Deflate before encrypting."),
					).Build()

					giu.Row(
						giu.Checkbox("Reed-Solomon", &reedsolo),
						giu.Tooltip("Prevent file corruption by erasure coding (slow)."),
						giu.Dummy(-170, 0),
						giu.Checkbox("Delete files", &delete),
						giu.Tooltip("Delete the input files after encryption."),
					).Build()

					giu.Row(
						giu.Checkbox("Split into chunks:", &split),
						giu.Tooltip("Split the output file into smaller chunks."),
						giu.Dummy(-170, 0),
						giu.InputText(&splitSize).Size(86/dpi).Flags(2).OnChange(func() {
							split = splitSize != ""
						}),
						giu.Tooltip("Choose the chunk size."),
						giu.Combo("##splitter", splitUnits[splitSelected], splitUnits, &splitSelected).Size(68),
						giu.Tooltip("Choose the chunk units."),
					).Build()
				} else {
					giu.Row(
						giu.Checkbox("Force decrypt", &keep),
						giu.Tooltip("Override security measures when decrypting."),
						giu.Dummy(-170, 0),
						giu.Checkbox("Delete volume", &delete),
						giu.Tooltip("Delete the volume after a successful decryption."),
					).Build()
				}
			}),

			giu.Label("Save output as:"),
			giu.Custom(func() {
				w, _ := giu.GetAvailableRegion()
				bw, _ := giu.CalcTextSize("Change")
				p, _ := giu.GetWindowPadding()
				bw += p * 2
				dw := w - bw - p
				giu.Style().SetDisabled(true).To(
					giu.InputText(func() *string {
						tmp := ""
						if outputFile == "" {
							return &tmp
						}
						tmp = filepath.Base(outputFile)
						return &tmp
					}()).Size(dw / dpi / dpi).Flags(16384),
				).Build()

				giu.SameLine()
				giu.Button("Change").Size(bw/dpi, 0).OnClick(func() {
					f := dialog.File().Title("Choose where to save the output. Don't include extensions.")
					f.SetStartDir(func() string {
						if len(onlyFiles) > 0 {
							return filepath.Dir(onlyFiles[0])
						}
						return filepath.Dir(onlyFolders[0])
					}())

					// Prefill the filename
					tmp := strings.TrimSuffix(filepath.Base(outputFile), ".pcv")
					f.SetInitFilename(strings.TrimSuffix(tmp, filepath.Ext(tmp)))
					if mode == "encrypt" && (len(allFiles) > 1 || len(onlyFolders) > 0 || compress) {
						f.SetInitFilename("Encrypted")
					}

					// Get the chosen file path
					file, err := f.Save()
					if file == "" || err != nil {
						return
					}

					// Add the correct extensions
					if mode == "encrypt" {
						if len(allFiles) > 1 || len(onlyFolders) > 0 || compress {
							file += ".zip.pcv"
						} else {
							file += filepath.Ext(inputFile) + ".pcv"
						}
					} else {
						if strings.HasSuffix(inputFile, ".zip.pcv") {
							file += ".zip"
						} else {
							tmp := strings.TrimSuffix(filepath.Base(inputFile), ".pcv")
							file += filepath.Ext(tmp)
						}
					}
					outputFile = file
					mainStatus = "Ready."
					mainStatusColor = WHITE
				}).Build()
				giu.Tooltip("Save the output with a custom name and path.").Build()
			}),

			giu.Dummy(0, 0),
			giu.Separator(),
			giu.Dummy(0, 0),
			giu.Button(startLabel).Size(giu.Auto, 34).OnClick(func() {
				if keyfile && keyfiles == nil {
					mainStatus = "Please select your keyfiles."
					mainStatusColor = RED
					return
				}
				tmp, err := strconv.Atoi(splitSize)
				if split && (splitSize == "" || tmp <= 0 || err != nil) {
					mainStatus = "Invalid split size."
					mainStatusColor = RED
					return
				}
				_, err = os.Stat(outputFile)
				if err == nil {
					showOverwrite = true
					modalId++
					giu.Update()
				} else {
					showProgress = true
					fastDecode = true
					canCancel = true
					modalId++
					giu.Update()
					go func() {
						work()
						working = false
						showProgress = false
						giu.Update()
					}()
				}
			}),
			giu.Style().SetColor(giu.StyleColorText, mainStatusColor).To(
				giu.Label(mainStatus),
			),
		),

		giu.Custom(func() {
			window.SetSize(int(318*dpi), giu.GetCursorPos().Y+1)
		}),
	)
}

func onDrop(names []string) {
	if showKeyfile {
		keyfiles = append(keyfiles, names...)

		// Remove duplicate keyfiles and make sure they're accessible
		var tmp []string
		for _, i := range keyfiles {
			duplicate := false
			for _, j := range tmp {
				if i == j {
					duplicate = true
				}
			}
			stat, _ := os.Stat(i)
			fin, err := os.Open(i)
			if err == nil {
				fin.Close()
			}
			if !duplicate && !stat.IsDir() && err == nil {
				tmp = append(tmp, i)
			}
		}
		keyfiles = tmp

		// Update the keyfile status
		if len(keyfiles) == 0 {
			keyfileLabel = "None selected."
		} else if len(keyfiles) == 1 {
			keyfileLabel = "Using 1 keyfile."
		} else {
			keyfileLabel = fmt.Sprintf("Using %d keyfiles.", len(keyfiles))
		}

		modalId++
		giu.Update()
		return
	}

	scanning = true
	files, folders, size := 0, 0, 0
	resetUI()

	// One item dropped
	if len(names) == 1 {
		stat, _ := os.Stat(names[0])

		// A folder was dropped
		if stat.IsDir() {
			folders++
			mode = "encrypt"
			inputLabel = "1 folder."
			startLabel = "Encrypt"
			onlyFolders = append(onlyFolders, names[0])
			inputFile = filepath.Join(filepath.Dir(names[0]), "Encrypted") + ".zip"
			outputFile = inputFile + ".pcv"
		} else { // A file was dropped
			files++

			// Is the file a part of a split volume?
			nums := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
			endsNum := false
			for _, i := range nums {
				if strings.HasSuffix(names[0], i) {
					endsNum = true
				}
			}
			isSplit := strings.Contains(names[0], ".pcv.") && endsNum

			// Decide if encrypting or decrypting
			if strings.HasSuffix(names[0], ".pcv") || isSplit {
				mode = "decrypt"
				inputLabel = "Volume for decryption."
				startLabel = "Decrypt"
				commentsLabel = "Comments (read-only):"
				commentsDisabled = true

				// Get the correct input and output filenames
				if isSplit {
					ind := strings.Index(names[0], ".pcv")
					names[0] = names[0][:ind+4]
					inputFile = names[0]
					outputFile = names[0][:ind]
					recombine = true

					totalFiles := 0
					// Find out the number of splitted chunks
					for {
						stat, err := os.Stat(fmt.Sprintf("%s.%d", inputFile, totalFiles))
						if err != nil {
							break
						}
						totalFiles++
						size += int(stat.Size())
					}
				} else {
					outputFile = names[0][:len(names[0])-4]
				}

				// Open the input file in read-only mode
				var fin *os.File
				var err error
				if isSplit {
					fin, err = os.Open(names[0] + ".0")
				} else {
					fin, err = os.Open(names[0])
				}
				if err != nil {
					resetUI()
					accessDenied("Read")
					return
				}

				// Use regex to test if the input is a valid Picocrypt volume
				tmp := make([]byte, 15)
				fin.Read(tmp)
				tmp, err = rsDecode(rs5, tmp)
				if valid, _ := regexp.Match(`^v\d\.\d{2}`, tmp); !valid || err != nil {
					resetUI()
					mainStatus = "This doesn't seem like a Picocrypt volume."
					mainStatusColor = RED
					fin.Close()
					return
				}

				// Read comments from file and check for corruption
				tmp = make([]byte, 15)
				fin.Read(tmp)
				tmp, err = rsDecode(rs5, tmp)
				if err == nil {
					commentsLength, _ := strconv.Atoi(string(tmp))
					tmp = make([]byte, commentsLength*3)
					fin.Read(tmp)
					comments = ""
					for i := 0; i < commentsLength*3; i += 3 {
						t, err := rsDecode(rs1, tmp[i:i+3])
						if err != nil {
							comments = "Comments are corrupted."
							break
						}
						comments += string(t)
					}
				} else {
					comments = "Comments are corrupted."
				}

				// Read flags from file and check for corruption
				flags := make([]byte, 15)
				fin.Read(flags)
				fin.Close()
				flags, err = rsDecode(rs5, flags)
				if err != nil {
					mainStatus = "The volume header is damaged."
					mainStatusColor = RED
					return
				}

				// Update UI and variables according to flags
				if flags[1] == 1 {
					keyfile = true
					keyfileLabel = "Keyfiles required."
				} else {
					keyfileLabel = "Not applicable."
				}
				if flags[2] == 1 {
					keyfileOrdered = true
				}
			} else { // One file was dropped for encryption
				mode = "encrypt"
				inputLabel = "1 file."
				startLabel = "Encrypt"
				inputFile = names[0]
				outputFile = names[0] + ".pcv"
			}

			// Add the file
			onlyFiles = append(onlyFiles, names[0])
			inputFile = names[0]
			if !isSplit {
				size += int(stat.Size())
			}
		}
	} else { // There are multiple dropped items
		mode = "encrypt"
		startLabel = "Encrypt"

		// Go through each dropped item and add to corresponding slices
		for _, name := range names {
			stat, _ := os.Stat(name)
			if stat.IsDir() {
				folders++
				onlyFolders = append(onlyFolders, name)
			} else {
				files++
				onlyFiles = append(onlyFiles, name)
				allFiles = append(allFiles, name)

				size += int(stat.Size())
				inputLabel = fmt.Sprintf("Scanning files... (%s)", sizeify(int64(size)))
				giu.Update()
			}
		}

		// Update UI with the number of files and folders selected
		if folders == 0 {
			inputLabel = fmt.Sprintf("%d files.", files)
		} else if files == 0 {
			inputLabel = fmt.Sprintf("%d folders.", folders)
		} else {
			if files == 1 && folders > 1 {
				inputLabel = fmt.Sprintf("1 file and %d folders.", folders)
			} else if folders == 1 && files > 1 {
				inputLabel = fmt.Sprintf("%d files and 1 folder.", files)
			} else if folders == 1 && files == 1 {
				inputLabel = "1 file and 1 folder."
			} else {
				inputLabel = fmt.Sprintf("%d files and %d folders.", files, folders)
			}
		}

		// Set the input and output paths
		inputFile = filepath.Join(filepath.Dir(names[0]), "Encrypted") + ".zip"
		outputFile = inputFile + ".pcv"
	}

	// Recursively add all files in 'onlyFolders' to 'allFiles'
	go func() {
		oldInputLabel := inputLabel
		for _, name := range onlyFolders {
			filepath.Walk(name, func(path string, _ os.FileInfo, _ error) error {
				stat, _ := os.Stat(path)
				if !stat.IsDir() {
					allFiles = append(allFiles, path)
					size += int(stat.Size())
					inputLabel = fmt.Sprintf("Scanning files... (%s)", sizeify(int64(size)))
					giu.Update()
				}
				return nil
			})
		}
		inputLabel = fmt.Sprintf("%s (%s)", oldInputLabel, sizeify(int64(size)))
		scanning = false
		giu.Update()
	}()
}

func work() {
	popupStatus = "Starting..."
	mainStatus = "Working..."
	mainStatusColor = WHITE
	working = true
	padded := false
	giu.Update()

	// Cryptography values
	var salt []byte                    // Argon2 salt, 16 bytes
	var hkdfSalt []byte                // HKDF-SHA3 salt, 32 bytes
	var serpentSalt []byte             // Serpent salt, 16 bytes
	var nonce []byte                   // 24-byte XChaCha20 nonce
	var keyHash []byte                 // SHA3-512 hash of encryption key
	var keyHashRef []byte              // Same as 'keyHash', but used for comparison
	var keyfileKey []byte              // The SHA3-256 hashes of keyfiles
	var keyfileHash = make([]byte, 32) // The SHA3-256 of 'keyfileKey'
	var keyfileHashRef []byte          // Same as 'keyfileHash', but used for comparison
	var authTag []byte                 // 64-byte authentication tag (BLAKE2b or HMAC-SHA3)

	// Combine/compress all files into a .zip file if needed
	if len(allFiles) > 1 || len(onlyFolders) > 0 || compress {
		// Consider case where compressing only one file
		files := allFiles
		if len(allFiles) == 0 {
			files = onlyFiles
		}

		// Get the root directory of the selected files
		var rootDir string
		if len(onlyFolders) > 0 {
			rootDir = filepath.Dir(onlyFolders[0])
		} else {
			rootDir = filepath.Dir(onlyFiles[0])
		}

		// Open a .zip file for writing
		inputFile = strings.TrimSuffix(outputFile, ".pcv")
		file, err := os.Create(inputFile)
		if err != nil {
			accessDenied("Write")
			return
		}

		// Calculate total size of uncompressed files
		compressTotal = 0
		for _, path := range files {
			stat, _ := os.Stat(path)
			compressTotal += stat.Size()
		}
		compressDone = 0

		writer := zip.NewWriter(file)
		compressStart = time.Now()

		// Add each file to the .zip

		for i, path := range files {
			progressInfo = fmt.Sprintf("%d/%d", i+1, len(files))
			giu.Update()

			// Create file info header (size, last modified, etc.)
			stat, _ := os.Stat(path)
			header, _ := zip.FileInfoHeader(stat)
			header.Name = strings.TrimPrefix(path, rootDir)
			header.Name = filepath.ToSlash(header.Name)
			header.Name = strings.TrimPrefix(header.Name, "/")

			if compress {
				header.Method = zip.Deflate
			} else {
				header.Method = zip.Store
			}

			// Open the file for reading
			entry, _ := writer.CreateHeader(header)
			fin, err := os.Open(path)
			if err != nil {
				writer.Close()
				file.Close()
				os.Remove(inputFile)
				resetUI()
				accessDenied("Read")
				return
			}

			// Use a passthrough to catch compression progress
			passthrough := &compressorProgress{Reader: fin}
			buf := make([]byte, MiB)
			_, err = io.CopyBuffer(entry, passthrough, buf)
			fin.Close()

			if err != nil {
				insufficientSpace()
				writer.Close()
				file.Close()
				os.Remove(inputFile)
				return
			}

			if !working {
				cancel()
				writer.Close()
				file.Close()
				os.Remove(inputFile)
				return
			}
		}
		writer.Close()
		file.Close()
	}

	// Recombine a split file if necessary
	if recombine {
		totalFiles := 0
		totalBytes := int64(0)
		done := 0

		// Find out the number of splitted chunks
		for {
			stat, err := os.Stat(fmt.Sprintf("%s.%d", inputFile, totalFiles))
			if err != nil {
				break
			}
			totalFiles++
			totalBytes += stat.Size()
		}

		// Create a .pcv to combine chunks into
		fout, err := os.Create(outputFile + ".pcv")
		if err != nil {
			accessDenied("Write")
			return
		}

		// Merge all chunks into one file
		startTime := time.Now()
		for i := 0; i < totalFiles; i++ {
			fin, _ := os.Open(fmt.Sprintf("%s.%d", inputFile, i))
			for {
				if !working {
					cancel()
					fin.Close()
					fout.Close()
					os.Remove(outputFile + ".pcv")
					return
				}

				// Copy from the chunk into the .pcv
				data := make([]byte, MiB)
				read, err := fin.Read(data)
				if err != nil {
					break
				}
				data = data[:read]
				_, err = fout.Write(data)
				done += read

				if err != nil {
					insufficientSpace()
					fin.Close()
					fout.Close()
					os.Remove(outputFile + ".pcv")
					return
				}

				// Update the stats
				progress, speed, eta = statify(int64(done), totalBytes, startTime)
				progressInfo = fmt.Sprintf("%d/%d", i+1, totalFiles)
				popupStatus = fmt.Sprintf("Recombining at %.2f MiB/s (ETA: %s)", speed, eta)
				giu.Update()
			}
			fin.Close()
		}
		fout.Close()
		inputFileOld = inputFile
		inputFile = outputFile + ".pcv"
	}

	canCancel = false
	progress = 0
	progressInfo = ""
	giu.Update()

	// Subtract the header size from the total size if decrypting
	stat, _ := os.Stat(inputFile)
	total := stat.Size()
	if mode == "decrypt" {
		total -= 789
	}

	// Open input file in read-only mode
	fin, err := os.Open(inputFile)
	if err != nil {
		resetUI()
		accessDenied("Read")
		return
	}

	// Set up output file
	var fout *os.File

	// If encrypting, generate values and write to file
	if mode == "encrypt" {
		popupStatus = "Generating values..."
		giu.Update()

		// Create the output file
		var err error
		fout, err = os.Create(outputFile)
		if err != nil {
			fin.Close()
			accessDenied("Write")
			return
		}

		// Set up cryptographic values
		salt = make([]byte, 16)
		hkdfSalt = make([]byte, 32)
		serpentSalt = make([]byte, 16)
		nonce = make([]byte, 24)

		// Write the program version to file
		fout.Write(rsEncode(rs5, []byte(version)))

		// Encode and write the comment length to file
		commentsLength := []byte(fmt.Sprintf("%05d", len(comments)))
		fout.Write(rsEncode(rs5, commentsLength))

		// Encode the comment and write to file
		for _, i := range []byte(comments) {
			fout.Write(rsEncode(rs1, []byte{i}))
		}

		// Configure flags and write to file
		flags := make([]byte, 5)
		if paranoid { // Paranoid mode selected
			flags[0] = 1
		}
		if len(keyfiles) > 0 { // Keyfiles are being used
			flags[1] = 1
		}
		if keyfileOrdered { // Order of keyfiles matter
			flags[2] = 1
		}
		if reedsolo { // Full Reed-Solomon encoding is selected
			flags[3] = 1
		}
		if total%int64(MiB) >= int64(MiB)-128 { // Reed-Solomon internals
			flags[4] = 1
		}
		fout.Write(rsEncode(rs5, flags))

		// Fill values with Go's CSPRNG
		rand.Read(salt)
		rand.Read(hkdfSalt)
		rand.Read(serpentSalt)
		rand.Read(nonce)

		// Encode values with Reed-Solomon and write to file
		fout.Write(rsEncode(rs16, salt))
		fout.Write(rsEncode(rs32, hkdfSalt))
		fout.Write(rsEncode(rs16, serpentSalt))
		fout.Write(rsEncode(rs24, nonce))

		// Write placeholders for future use
		fout.Write(make([]byte, 192)) // Hash of encryption key
		fout.Write(make([]byte, 96))  // Hash of keyfile key
		fout.Write(make([]byte, 192)) // BLAKE2b/HMAC-SHA3 tag
	} else { // Decrypting, read values from file and decode
		popupStatus = "Reading values..."
		giu.Update()

		// Stores any Reed-Solomon decoding errors
		errs := make([]error, 10)

		version := make([]byte, 15)
		fin.Read(version)
		_, errs[0] = rsDecode(rs5, version)

		tmp := make([]byte, 15)
		fin.Read(tmp)
		tmp, errs[1] = rsDecode(rs5, tmp)
		commentsLength, _ := strconv.Atoi(string(tmp))
		fin.Read(make([]byte, commentsLength*3))
		total -= int64(commentsLength) * 3

		flags := make([]byte, 15)
		fin.Read(flags)
		flags, errs[2] = rsDecode(rs5, flags)
		paranoid = flags[0] == 1
		reedsolo = flags[3] == 1
		padded = flags[4] == 1

		salt = make([]byte, 48)
		fin.Read(salt)
		salt, errs[3] = rsDecode(rs16, salt)

		hkdfSalt = make([]byte, 96)
		fin.Read(hkdfSalt)
		hkdfSalt, errs[4] = rsDecode(rs32, hkdfSalt)

		serpentSalt = make([]byte, 48)
		fin.Read(serpentSalt)
		serpentSalt, errs[5] = rsDecode(rs16, serpentSalt)

		nonce = make([]byte, 72)
		fin.Read(nonce)
		nonce, errs[6] = rsDecode(rs24, nonce)

		keyHashRef = make([]byte, 192)
		fin.Read(keyHashRef)
		keyHashRef, errs[7] = rsDecode(rs64, keyHashRef)

		keyfileHashRef = make([]byte, 96)
		fin.Read(keyfileHashRef)
		keyfileHashRef, errs[8] = rsDecode(rs32, keyfileHashRef)

		authTag = make([]byte, 192)
		fin.Read(authTag)
		authTag, errs[9] = rsDecode(rs64, authTag)

		// If there was an issue during decoding, the header is corrupted
		for _, err := range errs {
			if err != nil {
				if keep { // If the user chooses to force decrypt
					kept = true
				} else {
					mainStatus = "The volume header is damaged."
					mainStatusColor = RED
					fin.Close()
					if recombine {
						os.Remove(inputFile)
					}
					return
				}
			}
		}
	}

	popupStatus = "Deriving key..."
	giu.Update()

	// Derive encryption keys and subkeys
	var key []byte
	if paranoid {
		key = argon2.IDKey(
			[]byte(password),
			salt,
			8,     // 8 passes
			1<<20, // 1 GiB memory
			8,     // 8 threads
			32,    // 32-byte output key
		)
	} else {
		key = argon2.IDKey(
			[]byte(password),
			salt,
			4,
			1<<20,
			4,
			32,
		)
	}

	// If keyfiles are being used
	if len(keyfiles) > 0 || keyfile {
		popupStatus = "Reading keyfiles..."
		giu.Update()

		if keyfileOrdered { // If order matters, hash progressively
			var tmp = sha3.New256()
			for _, path := range keyfiles {
				fin, _ := os.Open(path)
				stat, _ := os.Stat(path)
				data := make([]byte, stat.Size())
				fin.Read(data)
				fin.Close()
				tmp.Write(data)
			}
			keyfileKey = tmp.Sum(nil)

			tmp = sha3.New256()
			tmp.Write(keyfileKey)
			keyfileHash = tmp.Sum(nil)
		} else { // If order doesn't matter, hash individually and combine
			for _, path := range keyfiles {
				fin, _ := os.Open(path)
				stat, _ := os.Stat(path)
				data := make([]byte, stat.Size())
				fin.Read(data)
				fin.Close()
				tmp := sha3.New256()
				tmp.Write(data)
				sum := tmp.Sum(nil)
				if keyfileKey == nil {
					keyfileKey = sum
				} else {
					for i, j := range sum {
						keyfileKey[i] ^= j
					}
				}
			}

			// Store a hash of the keyfile key for comparison
			tmp := sha3.New256()
			tmp.Write(keyfileKey)
			keyfileHash = tmp.Sum(nil)
		}
	}

	popupStatus = "Calculating values..."
	giu.Update()

	// Hash the encryption key for comparison when decrypting
	tmp := sha3.New512()
	tmp.Write(key)
	keyHash = tmp.Sum(nil)

	// Validate the password and/or keyfiles
	if mode == "decrypt" {
		keyCorrect := subtle.ConstantTimeCompare(keyHash, keyHashRef) == 1
		keyfileCorrect := subtle.ConstantTimeCompare(keyfileHash, keyfileHashRef) == 1
		incorrect := !keyCorrect
		if keyfile {
			incorrect = !keyCorrect || !keyfileCorrect
		}

		// If something is incorrect
		if incorrect {
			if keep {
				kept = true
			} else {
				if !keyCorrect {
					mainStatus = "The provided password is incorrect."
				} else {
					if keyfileOrdered {
						mainStatus = "Incorrect keyfiles or order."
					} else {
						mainStatus = "Incorrect keyfiles."
					}
				}
				mainStatusColor = RED
				fin.Close()
				if recombine {
					os.Remove(inputFile)
				}
				return
			}
		}

		// Create the output file for decryption
		fout, err = os.Create(outputFile)
		if err != nil {
			fin.Close()
			accessDenied("Write")
			return
		}
	}

	if len(keyfiles) > 0 || keyfile {
		// XOR the encryption key with the keyfile key
		tmp := key
		key = make([]byte, 32)
		for i := range key {
			key[i] = tmp[i] ^ keyfileKey[i]
		}
	}

	done, counter := 0, 0
	chacha, _ := chacha20.NewUnauthenticatedCipher(key, nonce)

	// Use HKDF-SHA3 to generate a subkey
	var mac hash.Hash
	subkey := make([]byte, 32)
	hkdf := hkdf.New(sha3.New256, key, hkdfSalt, nil)
	hkdf.Read(subkey)
	if paranoid {
		mac = hmac.New(sha3.New512, subkey) // HMAC-SHA3
	} else {
		mac, _ = blake2b.New512(subkey) // Keyed BLAKE2b
	}

	// Generate another subkey for use as Serpent's salt
	serpentKey := make([]byte, 32)
	hkdf.Read(serpentKey)
	s, _ := serpent.NewCipher(serpentKey)
	serpent := cipher.NewCTR(s, serpentSalt)

	canCancel = true
	startTime := time.Now()
	for {
		// If the user cancels the process, stop and clean up
		if !working {
			cancel()
			fin.Close()
			fout.Close()
			if recombine || len(allFiles) > 1 || len(onlyFolders) > 0 || compress {
				os.Remove(inputFile)
			}
			os.Remove(outputFile)
			return
		}

		// Read in data from the file
		var src []byte
		if mode == "decrypt" && reedsolo {
			src = make([]byte, MiB/128*136)
		} else {
			src = make([]byte, MiB)
		}
		size, err := fin.Read(src)
		if err != nil {
			break
		}
		src = src[:size]
		dst := make([]byte, len(src))

		// Do the actual encryption
		if mode == "encrypt" {
			if paranoid {
				serpent.XORKeyStream(dst, src)
				copy(src, dst)
			}

			chacha.XORKeyStream(dst, src)
			mac.Write(dst)

			if reedsolo {
				copy(src, dst)
				dst = nil
				// If a full MiB is available
				if len(src) == MiB {
					// Encode every chunk
					for i := 0; i < MiB; i += 128 {
						dst = append(dst, rsEncode(rs128, src[i:i+128])...)
					}
				} else {
					// Encode the full chunks
					chunks := math.Floor(float64(len(src)) / 128)
					for i := 0; float64(i) < chunks; i++ {
						dst = append(dst, rsEncode(rs128, src[i*128:(i+1)*128])...)
					}

					// Pad and encode the final partial chunk
					dst = append(dst, rsEncode(rs128, pad(src[int(chunks*128):]))...)
				}
			}
		} else { // Decryption
			if reedsolo {
				copy(dst, src)
				src = nil
				// If a complete 1 MiB block is available
				if len(dst) == MiB/128*136 {
					// Decode every chunk
					for i := 0; i < MiB/128*136; i += 136 {
						tmp, err := rsDecode(rs128, dst[i:i+136])
						if err != nil {
							if keep {
								kept = true
							} else {
								broken(fin, fout, "The input file is irrecoverably damaged.")
								return
							}
						}
						if i == MiB/128*136-136 && done+MiB/128*136 >= int(total) && padded {
							tmp = unpad(tmp)
						}
						src = append(src, tmp...)

						if !fastDecode && i%17408 == 0 {
							progress, speed, eta = statify(int64(done+i), total, startTime)
							progressInfo = fmt.Sprintf("%.2f%%", progress*100)
							popupStatus = fmt.Sprintf("Repairing at %.2f MiB/s (ETA: %s)", speed, eta)
							giu.Update()
						}
					}
				} else {
					// Decode the full chunks
					chunks := len(dst)/136 - 1
					for i := 0; i < chunks; i++ {
						tmp, err := rsDecode(rs128, dst[i*136:(i+1)*136])
						if err != nil {
							if keep {
								kept = true
							} else {
								broken(fin, fout, "The input file is irrecoverably damaged.")
								return
							}
						}
						src = append(src, tmp...)

						if !fastDecode && i%128 == 0 {
							progress, speed, eta = statify(int64(done+i*136), total, startTime)
							progressInfo = fmt.Sprintf("%.2f%%", progress*100)
							popupStatus = fmt.Sprintf("Repairing at %.2f MiB/s (ETA: %s)", speed, eta)
							giu.Update()
						}
					}

					// Unpad and decode the final partial chunk
					tmp, err := rsDecode(rs128, dst[int(chunks)*136:])
					if err != nil {
						if keep {
							kept = true
						} else {
							broken(fin, fout, "The input file is irrecoverably damaged.")
							return
						}
					}
					src = append(src, unpad(tmp)...)
				}
				dst = make([]byte, len(src))
			}

			mac.Write(src)
			chacha.XORKeyStream(dst, src)

			if paranoid {
				copy(src, dst)
				serpent.XORKeyStream(dst, src)
			}
		}

		_, err = fout.Write(dst)
		if err != nil {
			insufficientSpace()
			fin.Close()
			fout.Close()
			if recombine || len(allFiles) > 1 || len(onlyFolders) > 0 || compress {
				os.Remove(inputFile)
			}
			os.Remove(outputFile)
			return
		}

		// Update stats
		if mode == "decrypt" && reedsolo {
			done += MiB / 128 * 136
		} else {
			done += MiB
		}
		counter += MiB
		progress, speed, eta = statify(int64(done), total, startTime)
		progressInfo = fmt.Sprintf("%.2f%%", progress*100)
		if mode == "encrypt" {
			popupStatus = fmt.Sprintf("Encrypting at %.2f MiB/s (ETA: %s)", speed, eta)
		} else {
			if fastDecode {
				popupStatus = fmt.Sprintf("Decrypting at %.2f MiB/s (ETA: %s)", speed, eta)
			}
		}
		giu.Update()

		// Change counters after 60 GiB to prevent overflow
		if counter >= 60*GiB {
			// ChaCha20
			nonce = make([]byte, 24)
			hkdf.Read(nonce)
			chacha, _ = chacha20.NewUnauthenticatedCipher(key, nonce)

			// Serpent
			serpentSalt = make([]byte, 16)
			hkdf.Read(serpentSalt)
			serpent = cipher.NewCTR(s, serpentSalt)

			// Reset counter to 0
			counter = 0
		}
	}

	progress = 0
	progressInfo = ""
	giu.Update()

	if mode == "encrypt" {
		popupStatus = "Writing values..."
		giu.Update()

		// Seek back to header and write important values
		fout.Seek(int64(309+len(comments)*3), 0)
		fout.Write(rsEncode(rs64, keyHash))
		fout.Write(rsEncode(rs32, keyfileHash))
		fout.Write(rsEncode(rs64, mac.Sum(nil)))
	} else {
		popupStatus = "Comparing values..."
		giu.Update()

		// Validate the authenticity of decrypted data
		if subtle.ConstantTimeCompare(mac.Sum(nil), authTag) == 0 {
			if reedsolo && fastDecode {
				fastDecode = false
				fin.Close()
				fout.Close()
				work()
				return
			}
			if keep {
				kept = true
			} else {
				broken(fin, fout, "The input file is damaged or modified.")
				return
			}
		}
	}

	fin.Close()
	fout.Close()

	// Split the file into chunks
	if split {
		var splitted []string
		stat, _ := os.Stat(outputFile)
		size := stat.Size()
		finishedFiles := 0
		finishedBytes := 0
		chunkSize, _ := strconv.Atoi(splitSize)

		// Calculate chunk size
		if splitSelected == 0 {
			chunkSize *= KiB
		} else if splitSelected == 1 {
			chunkSize *= MiB
		} else if splitSelected == 2 {
			chunkSize *= GiB
		} else if splitSelected == 3 {
			chunkSize *= TiB
		} else {
			chunkSize = int(math.Ceil(float64(size) / float64(chunkSize)))
		}

		// Get the number of required chunks
		chunks := int(math.Ceil(float64(size) / float64(chunkSize)))
		progressInfo = fmt.Sprintf("%d/%d", finishedFiles+1, chunks)
		giu.Update()
		fin, _ := os.Open(outputFile)

		startTime := time.Now()
		for i := 0; i < chunks; i++ {
			// Make the chunk
			fout, _ := os.Create(fmt.Sprintf("%s.%d", outputFile, i))
			done := 0

			// Copy data into the chunk
			for {
				data := make([]byte, MiB)
				for done+len(data) > chunkSize {
					data = make([]byte, int(math.Ceil(float64(len(data))/2)))
				}

				read, err := fin.Read(data)
				if err != nil {
					break
				}
				if !working {
					cancel()
					fin.Close()
					fout.Close()
					if len(allFiles) > 1 || len(onlyFolders) > 0 || compress {
						os.Remove(inputFile)
					}
					os.Remove(outputFile)

					// If user cancels, remove the unfinished files
					for _, j := range splitted {
						os.Remove(j)
					}
					os.Remove(fmt.Sprintf("%s.%d", outputFile, i))

					return
				}

				data = data[:read]
				_, err = fout.Write(data)
				if err != nil {
					insufficientSpace()
					fin.Close()
					fout.Close()
					if len(allFiles) > 1 || len(onlyFolders) > 0 || compress {
						os.Remove(inputFile)
					}
					os.Remove(outputFile)

					// If user cancels, remove the unfinished files
					for _, j := range splitted {
						os.Remove(j)
					}
					os.Remove(fmt.Sprintf("%s.%d", outputFile, i))

					return
				}
				done += read
				if done >= chunkSize {
					break
				}

				// Update stats
				finishedBytes += read
				progress, speed, eta = statify(int64(finishedBytes), int64(size), startTime)
				popupStatus = fmt.Sprintf("Splitting at %.2f MiB/s (ETA: %s)", speed, eta)
				giu.Update()
			}
			fout.Close()

			// Update stats
			finishedFiles++
			if finishedFiles == chunks {
				finishedFiles--
			}
			splitted = append(splitted, fmt.Sprintf("%s.%d", outputFile, i))
			progressInfo = fmt.Sprintf("%d/%d", finishedFiles+1, chunks)
			giu.Update()
		}

		fin.Close()
		os.Remove(outputFile)
	}

	canCancel = false
	progress = 0
	progressInfo = ""
	giu.Update()

	// Remove the temporary file used to combine a splitted volume
	if recombine {
		os.Remove(inputFile)
	}

	// Delete the temporary .zip used to encrypt files
	if len(allFiles) > 1 || len(onlyFolders) > 0 || compress {
		os.Remove(inputFile)
	}

	// Delete the input files if the user chooses
	if delete {
		popupStatus = "Deleting files..."
		giu.Update()

		if mode == "decrypt" {
			if recombine { // Remove each chunk
				i := 0
				for {
					_, err := os.Stat(fmt.Sprintf("%s.%d", inputFileOld, i))
					if err != nil {
						break
					}
					os.Remove(fmt.Sprintf("%s.%d", inputFileOld, i))
					i++
				}
			} else {
				os.Remove(inputFile)
			}
		} else {
			for _, i := range onlyFiles {
				os.Remove(i)
			}
			for _, i := range onlyFolders {
				os.RemoveAll(i)
			}
		}
	}

	// All done, reset the UI
	oldKept := kept
	resetUI()
	kept = oldKept

	// If the user chose to keep a corrupted/modified file, let them know
	if kept {
		mainStatus = "The input file was modified. Please be careful."
		mainStatusColor = YELLOW
	} else {
		mainStatus = "Completed."
		mainStatusColor = GREEN
	}
}

// If the OS denies reading or writing to a file
func accessDenied(s string) {
	mainStatus = s + " access denied by operating system."
	mainStatusColor = RED
}

// If there isn't enough disk space
func insufficientSpace() {
	mainStatus = "Insufficient disk space."
	mainStatusColor = RED
}

// If corruption is detected during decryption
func broken(fin *os.File, fout *os.File, message string) {
	fin.Close()
	fout.Close()
	mainStatus = message
	mainStatusColor = RED

	// Clean up files since decryption failed
	if recombine {
		os.Remove(inputFile)
	}
	os.Remove(outputFile)
}

// Stop working
func cancel() {
	mainStatus = "Operation cancelled by user."
	mainStatusColor = WHITE
}

// Reset the UI to a clean state with nothing selected or checked
func resetUI() {
	imgui.ClearActiveID()
	mode = ""

	inputFile = ""
	inputFileOld = ""
	outputFile = ""
	onlyFiles = nil
	onlyFolders = nil
	allFiles = nil
	inputLabel = "Drop files and folders into this window."

	password = ""
	cpassword = ""
	passwordState = giu.InputTextFlagsPassword
	passwordStateLabel = "Show"

	passgenLength = 32
	passgenUpper = true
	passgenLower = true
	passgenNums = true
	passgenSymbols = true
	passgenCopy = true

	keyfile = false
	keyfiles = nil
	keyfileOrdered = false
	keyfileLabel = "None selected."

	comments = ""
	commentsLabel = "Comments:"
	commentsDisabled = false

	paranoid = false
	reedsolo = false
	split = false
	splitSize = ""
	splitSelected = 1
	recombine = false
	compress = false
	delete = false
	keep = false
	kept = false

	startLabel = "Start"
	mainStatus = "Ready."
	mainStatusColor = WHITE
	popupStatus = ""

	progress = 0
	progressInfo = ""
	giu.Update()
}

// Reed-Solomon encoder
func rsEncode(rs *infectious.FEC, data []byte) []byte {
	res := make([]byte, rs.Total())
	rs.Encode(data, func(s infectious.Share) {
		res[s.Number] = s.Data[0]
	})
	return res
}

// Reed-Solomon decoder
func rsDecode(rs *infectious.FEC, data []byte) ([]byte, error) {
	// If fast decode, just return the first 128 bytes
	if rs.Total() == 136 && fastDecode {
		return data[:128], nil
	}

	tmp := make([]infectious.Share, rs.Total())
	for i := 0; i < rs.Total(); i++ {
		tmp[i].Number = i
		tmp[i].Data = append(tmp[i].Data, data[i])
	}
	res, err := rs.Decode(nil, tmp)

	// Force decode the data but return the error as well
	if err != nil {
		if rs.Total() == 136 {
			return data[:128], err
		}
		return data[:rs.Total()/3], err
	}
	return res, nil
}

// PKCS#7 pad (for use with Reed-Solomon)
func pad(data []byte) []byte {
	padLen := 128 - len(data)%128
	padding := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, padding...)
}

// PKCS#7 unpad
func unpad(data []byte) []byte {
	padLen := int(data[127])
	return data[:128-padLen]
}

// Generate a cryptographically secure password
func genPassword() string {
	chars := ""
	if passgenUpper {
		chars += "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	}
	if passgenLower {
		chars += "abcdefghijklmnopqrstuvwxyz"
	}
	if passgenNums {
		chars += "1234567890"
	}
	if passgenSymbols {
		chars += "-=_+!@#$^&()?<>"
	}
	tmp := make([]byte, passgenLength)
	for i := 0; i < int(passgenLength); i++ {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		tmp[i] = chars[j.Int64()]
	}
	if passgenCopy {
		clipboard.WriteAll(string(tmp))
	}
	return string(tmp)
}

// Convert done, total, and starting time to progress, speed, and ETA
func statify(done int64, total int64, start time.Time) (float32, float64, string) {
	progress := float32(done) / float32(total)
	elapsed := float64(time.Since(start)) / float64(MiB) / 1000
	speed := float64(done) / elapsed / float64(MiB)
	eta := int(math.Floor(float64(total-done) / (speed * float64(MiB))))
	return float32(math.Min(float64(progress), 1)), speed, timeify(eta)
}

// Convert seconds to HH:MM:SS
func timeify(seconds int) string {
	hours := int(math.Floor(float64(seconds) / 3600))
	seconds %= 3600
	minutes := int(math.Floor(float64(seconds) / 60))
	seconds %= 60
	hours = int(math.Max(float64(hours), 0))
	minutes = int(math.Max(float64(minutes), 0))
	seconds = int(math.Max(float64(seconds), 0))
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// Convert bytes to KiB, MiB, etc.
func sizeify(size int64) string {
	if size >= int64(TiB) {
		return fmt.Sprintf("%.2f TiB", float64(size)/float64(TiB))
	} else if size >= int64(GiB) {
		return fmt.Sprintf("%.2f GiB", float64(size)/float64(GiB))
	} else if size >= int64(MiB) {
		return fmt.Sprintf("%.2f MiB", float64(size)/float64(MiB))
	} else {
		return fmt.Sprintf("%.2f KiB", float64(size)/float64(KiB))
	}
}

func main() {
	// Create the main window
	window = giu.NewMasterWindow("Picocrypt", 318, 479, giu.MasterWindowFlagsNotResizable)

	// Start the dialog module
	dialog.Init()

	// Set callbacks
	window.SetDropCallback(onDrop)
	window.SetCloseCallback(func() bool {
		return !working && !showProgress
	})

	// Set universal DPI
	dpi = giu.Context.GetPlatform().GetContentScale()

	// Start the UI
	window.Run(draw)
}
