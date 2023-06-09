package main

import (
	"errors"
	"path/filepath"
	"strings"
	"syscall/js" // for wasm
	"time"

	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	storagefs "github.com/go-git/go-git/v5/storage/filesystem"
)

var repoLocation = "wasm-repo"
var Filesystem = memfs.New()

type fs struct {
	storage billy.Filesystem
}

func ( f *fs ) createFile() js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		var file string

		if len(args) > 0 {
			file = args[0].String()
		}

		handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			resolv := args[0]
			reject := args[1]

			go func() {
				storage := f.storage
				_, err 	:= storage.Create(file)
				if err == nil {
					resolv.Invoke("File created")	
				} else {
					err = errors.New("file not created")
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
				}
			}()
			return nil
		})

		return js.Global().Get("Promise").New(handler)
	})
}

func ( f *fs ) ls() js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		var dir string

		if len(args) > 0 {
			dir = args[0].String()
		}

		handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			resolv := args[0]
			reject := args[1]

			go func() {
				storage 	:= f.storage
				files, err 	:= storage.ReadDir(dir)
				if err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
				} else {
					// create a new array
					filesArray 	:= js.Global().Get("Array").New(len(files))
					for i, file := range files {
						filesArray.SetIndex(i, file.Name())
					}
					resolv.Invoke(filesArray)
				}
			}()
			return nil
		})

		return js.Global().Get("Promise").New(handler)
	})
}

func ( f *fs ) writeNewFile() js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		var file string
		var content string

		if len(args) > 0 {
			file = args[0].String()
			content = args[1].String()
		}

		handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			resolv := args[0]
			reject := args[1]

			go func() {
				storage := f.storage
				f, err := storage.Create(file)
				if err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
				} else {
					_, err = f.Write([]byte(content))
					if err != nil {
						reject.Invoke(js.Global().Get("Error").New(err.Error()))
					} else {
						resolv.Invoke("File created")
					}
				}
			}()
			return nil
		})

		return js.Global().Get("Promise").New(handler)
	})
}


func git_clone() js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		var url string
		if len(args) > 0 {
			url = args[0].String()
		}

		workTreeFs, _ 	:= Filesystem.Chroot(repoLocation)
		dotGitFs, _ 	:= Filesystem.Chroot(filepath.Join(repoLocation, ".git"))
		storage 		:= storagefs.NewStorage(dotGitFs, cache.NewObjectLRUDefault())

		handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			resolv := args[0]
			reject := args[1]

			go func() {
				_, repoErr := gogit.Clone(storage, workTreeFs, &gogit.CloneOptions{
					URL: url,
				})

				if repoErr != nil {
					reject.Invoke(js.Global().Get("Error").New(repoErr.Error()))
				} else {
					resolv.Invoke("Repo cloned")
				}
			}()
			return nil
		})

		return js.Global().Get("Promise").New(handler)
	})
}

func git_push() js.Func {
	/*
		takes:
			url				# url of the repo (requires CORS to be enabled)
			AccessTocken	# access token for the with read/write access to the repo
			username		# username of the committer (required for better git history)
			email			# email of the committer    (required for better git history)
			file to push	# file to push
			commit message	# commit message
	*/
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		var url 		string
		var accessToken string
		var username 	string
		var email 		string
		var file 		string
		var commitMessage string

		if len(args) > 0 {
			url 			= args[0].String()
			accessToken 	= args[1].String()
			username 		= args[2].String()
			email 			= args[3].String()
			file 			= args[4].String()
			commitMessage 	= args[5].String()
		}

		workTreeFs, _ 	:= Filesystem.Chroot(repoLocation)
		dotGitFs, _ 	:= Filesystem.Chroot(filepath.Join(repoLocation, ".git"))
		storage 		:= storagefs.NewStorage(dotGitFs, cache.NewObjectLRUDefault())

		handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			resolv := args[0]
			reject := args[1]

			go func() {
				repo, repoErr := gogit.Open(storage, workTreeFs)
				if repoErr != nil {
					reject.Invoke(js.Global().Get("Error").New(repoErr.Error()))
				}

				// get the working directory for the repository
				w, wErr := repo.Worktree()
				if wErr != nil {
					reject.Invoke(js.Global().Get("Error").New(wErr.Error()))
				}

				// add all files
				_, addErr := w.Add(file)
				if addErr != nil {
					reject.Invoke(js.Global().Get("Error").New(addErr.Error()))
				}

				// commit all changes
				_, commitErr := w.Commit(commitMessage, &gogit.CommitOptions{
					Author: &object.Signature{
						Name:  username,
						Email: email,
						When:  time.Now(),
					},
				})
				if commitErr != nil {
					reject.Invoke(js.Global().Get("Error").New(commitErr.Error()))
				}

				// push all changes
				pushErr := repo.Push(&gogit.PushOptions{
					RemoteURL: url,
					Auth: &http.BasicAuth{
						Username: username,
						Password: accessToken,
					},
				})
				if pushErr != nil {
					reject.Invoke(js.Global().Get("Error").New(pushErr.Error()))
				} else {
					resolv.Invoke("Pushed")
				}
			}()
			return nil
		})
		return js.Global().Get("Promise").New(handler)
	})
}

func encrypt_text() js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		var text string
		var key  string

		if len(args) > 0 {
			text = args[0].String()
			key  = args[1].String() // 32 bytes key hex encoded
		}
		
		handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			resolv := args[0]
			reject := args[1]

			go func() {
				// remove "U+0022" from the start and end of the string
				key = strings.TrimPrefix(key, "\"")

				key, err := hex.DecodeString(key)
				if err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
				}

				block, err := aes.NewCipher(key)
				if err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
				}

				stream := cipher.NewCTR(block, key[:block.BlockSize()])
				ciphertext := make([]byte, len(text))
				stream.XORKeyStream(ciphertext, []byte(text))

				hexEncoded := hex.EncodeToString(ciphertext)
				resolv.Invoke(hexEncoded)
			}()

			return nil
		})
		return js.Global().Get("Promise").New(handler)
	})
}

func decrypt_text() js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		var text string
		var key  string

		if len(args) > 0 {
			text = args[0].String()	// cypher text hex encoded
			key  = args[1].String() // 32 bytes key hex encoded
		}

		handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			resolv := args[0]
			reject := args[1]

			go func() {
				// remove "U+0022" from the start and end of the string
				text = strings.TrimPrefix(text, "\"")
				key = strings.TrimPrefix(key, "\"")

				text, err := hex.DecodeString(text)
				if err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
				}

				key, err := hex.DecodeString(key)
				if err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
				}

				block, err := aes.NewCipher(key)
				if err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
				}

				stream := cipher.NewCTR(block, key[:block.BlockSize()])
				plaintext := make([]byte, len(text))
				stream.XORKeyStream(plaintext, text)

				resolv.Invoke(string(plaintext))
			}()
			
			return nil
		})

		return js.Global().Get("Promise").New(handler)
	})
}

func regiterCallbacks() {
	js.Global().Set("git_clone", git_clone())
	js.Global().Set("git_push", git_push())
	js.Global().Set("encrypt_text", encrypt_text())
	js.Global().Set("decrypt_text", decrypt_text())

	js.Global().Set("createFile", (&fs{storage: Filesystem}).createFile())
	js.Global().Set("ls", (&fs{storage: Filesystem}).ls())
	js.Global().Set("touchNcat", (&fs{storage: Filesystem}).writeNewFile())
}


func main() {
	regiterCallbacks() 	// register go functions to be called from js
	select {} 			// block forever
}
