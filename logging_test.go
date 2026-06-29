package logging

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	panicMsg   = "This is a PANIC message"
	errorMsg   = "This is an ERROR message"
	warningMsg = "This is a WARNING message"
	infoMsg    = "This is an INFO message"
	debugMsg   = "This is a DEBUG message"
)

type customPrefix struct {
	prefixFormat string
	currentFile  string
}

func (cp *customPrefix) CreatePrefix(loggingLevel Level) string {
	return fmt.Sprintf(cp.prefixFormat, loggingLevel, GetLogLevel(), cp.currentFile)
}

func (cp *customPrefix) CreateStructuredPrefix(loggingLevel Level, message string) []interface{} {
	return []interface{}{
		"custom-level", loggingLevel,
		"custom-file", cp.currentFile,
		"custom-message", message,
	}
}

func TestLogging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CNI-LOG Test Suite")
}

var _ = Describe("CNI Logging Operations", func() {
	var logFile string
	var logFileName string
	var tmpDir string

	BeforeEach(func() {
		initLogger()
		tmpDir = GinkgoT().TempDir()
		SetLogFileBaseDir(tmpDir)
		logFileName = "test.log"
		logFile = path.Join(tmpDir, logFileName)
	})

	Context("Default settings", func() {
		When("the defaults are used", func() {
			It("logs to stderr", func() {
				Expect(logToStderr).To(BeTrue())
			})

			It("does not log to file", func() {
				Expect(isFileLoggingEnabled()).To(BeFalse())
			})
		})
	})

	Context("Setting error logging", func() {
		Context("File logging is disabled", func() {
			When("error logging is enabled first and file logging is disabled later", func() {
				It("does not report an error", func() {
					errStr := captureStdErr(SetLogStderr, true)
					Expect(errStr).To(BeEmpty())
					errStr = captureStdErr(SetLogFile, "")
					Expect(errStr).To(BeEmpty())
				})
			})

			When("error logging is disabled while file logging is disabled", func() {
				It("does report an error", func() {
					errStr := captureStdErr(SetLogFile, "")
					Expect(errStr).To(BeEmpty())
					errStr = captureStdErr(SetLogStderr, false)
					Expect(errStr).To(ContainSubstring(logFileReqFailMsg))
				})
			})
		})

		Context("File logging is enabled", func() {
			When("error logging is enabled first and file logging is enabled later", func() {
				It("does not report an error", func() {
					errStr := captureStdErr(SetLogStderr, true)
					Expect(errStr).To(BeEmpty())
					errStr = captureStdErr(SetLogFile, logFileName)
					Expect(errStr).To(BeEmpty())
				})
			})

			When("error logging is disabled while file logging is enabled", func() {
				It("does not report an error", func() {
					errStr := captureStdErr(SetLogFile, logFileName)
					Expect(errStr).To(BeEmpty())
					errStr = captureStdErr(SetLogStderr, false)
					Expect(errStr).To(BeEmpty())
				})
			})
		})
	})

	Context("Setting the log file name", func() {
		When("the log file name is empty", func() {
			It("an error to standard output is thrown when logging to stderr is off", func() {
				errStr := captureStdErr(SetLogStderr, false)
				Expect(errStr).To(ContainSubstring(logFileReqFailMsg))
				errStr = captureStdErr(SetLogFile, "")
				Expect(errStr).To(ContainSubstring(logFileReqFailMsg))
			})

			It("no error to standard output is thrown when logging to stderr is on", func() {
				errStr := captureStdErr(SetLogStderr, true)
				Expect(errStr).To(BeEmpty())
				errStr = captureStdErr(SetLogFile, "")
				Expect(errStr).To(BeEmpty())
			})
		})

		When("the log file name is valid", func() {
			It("prepares the logger's writer and creates the log file", func() {
				SetLogFile(logFileName)
				Expect(logWriter).To(Equal(logger))
				Expect(logFile).To(BeAnExistingFile())
			})
		})

		When("the log file's parent directory does not exist", func() {
			BeforeEach(func() {
				logFileName = path.Join("nested", "nested", "test.log")
				logFile = path.Join(tmpDir, logFileName)
			})

			It("should be created", func() {
				SetLogFile(logFileName)
				Expect(logWriter).To(Equal(logger))
				Expect(logFile).To(BeAnExistingFile())
			})
		})

		When("the log file name is invalid", func() {
			It("an error to standard output is thrown", func() {
				filename := "/proc/foobar.log"
				expectedLoggerOutput := fmt.Sprintf(logFileFailMsgWithError, filename, absolutePathsFailMsg)
				loggerOutput := captureStdErr(SetLogFile, filename)
				Expect(loggerOutput).To(Equal(expectedLoggerOutput))
			})
		})

		When("the log file is set to a symbolic link", func() {
			var symlinkTarget string
			var symlinkName string
			var baseDir string

			BeforeEach(func() {
				baseDir = GinkgoT().TempDir()
				SetLogFileBaseDir(baseDir)

				symlinkTarget = path.Join(baseDir, "symlink-target-dir")
				symlinkName = "symtarget.txt"

				err := os.MkdirAll(symlinkTarget, 0755)
				Expect(err).ToNot(HaveOccurred())

				err = os.Symlink(symlinkTarget, path.Join(baseDir, symlinkName))
				Expect(err).ToNot(HaveOccurred())
			})

			It("an error to standard error is thrown", func() {
				expectedLoggerOutput := fmt.Sprintf(logFileFailMsgWithError, symlinkName, symlinkEvalFailMsg)
				loggerOutput := captureStdErr(SetLogFile, symlinkName)
				Expect(loggerOutput).To(Equal(expectedLoggerOutput))
			})
		})
	})

	Context("Setting the log options", func() {
		When("the logOption's fields are all populated", func() {
			It("logOptions should be set correctly", func() {
				expectedLogger := &lumberjack.Logger{
					Filename:   logFile,
					MaxAge:     1,
					MaxSize:    10,
					MaxBackups: 1,
					Compress:   true,
				}

				SetLogFile(logFileName)
				logOpts := &LogOptions{
					MaxAge:     getPrimitivePointer(1),
					MaxSize:    getPrimitivePointer(10),
					MaxBackups: getPrimitivePointer(1),
					Compress:   getPrimitivePointer(true),
				}
				SetLogOptions(logOpts)
				Expect(logger).To(Equal(expectedLogger))
			})
		})

		When("there are some fields missing", func() {
			It("should provide default values to the missing fields", func() {
				expectedLogger := &lumberjack.Logger{
					Filename:   logFile,
					MaxAge:     5,
					MaxSize:    100,
					MaxBackups: 1,
					Compress:   true,
				}
				SetLogFile(logFileName)
				logOpts := &LogOptions{
					MaxBackups: getPrimitivePointer(1),
					Compress:   getPrimitivePointer(true),
				}
				SetLogOptions(logOpts)
				Expect(logger).To(Equal(expectedLogger))
			})
		})

		When("logOptions isn't set at all", func() {
			It("should provide a default logOptions", func() {
				SetLogFile(logFileName)
				expectedLogger := &lumberjack.Logger{
					Filename:   logFile,
					MaxAge:     5,
					MaxSize:    100,
					MaxBackups: 5,
					Compress:   true,
				}

				SetLogOptions(nil)
				Expect(logger).To(Equal(expectedLogger))
			})
		})
	})

	Context("Logging messages", Ordered, func() {
		When("log level is set to ERROR", Ordered, func() {
			It("should print appropriate >= error messages to log file", func() {
				SetLogFile(logFileName)
				SetLogLevel(StringToLevel(errorStr))
				SetLogStderr(false)

				Panicf(panicMsg)
				Expect(logFileContains(logFile, panicMsg)).To(BeTrue())
				_ = Errorf(errorMsg)
				Expect(logFileContains(logFile, errorMsg)).To(BeTrue())
				Warningf(warningMsg)
				Expect(logFileContains(logFile, warningMsg)).To(BeFalse())
				Infof(infoMsg)
				Expect(logFileContains(logFile, infoMsg)).To(BeFalse())
				Debugf(debugMsg)
				Expect(logFileContains(logFile, debugMsg)).To(BeFalse())
			})

			It("should print appropriate >= error structured messages to log file", func() {
				SetLogFile(logFileName)
				SetLogLevel(StringToLevel(errorStr))
				SetLogStderr(false)

				PanicStructured(panicMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, panicStr, panicMsg))).To(BeTrue())
				_ = ErrorStructured(errorMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, errorStr, errorMsg))).To(BeTrue())
				WarningStructured(warningMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, warningStr, warningMsg))).To(BeFalse())
				InfoStructured(infoMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, infoStr, infoMsg))).To(BeFalse())
				DebugStructured(debugMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, debugStr, debugMsg))).To(BeFalse())
			})
		})

		When("log level is set to INFO", func() {
			It("should print appropriate >= info messages to log file", func() {
				SetLogFile(logFileName)
				SetLogLevel(StringToLevel(infoStr))
				SetLogStderr(false)

				Panicf(panicMsg)
				Expect(logFileContains(logFile, panicMsg)).To(BeTrue())
				_ = Errorf(errorMsg)
				Expect(logFileContains(logFile, errorMsg)).To(BeTrue())
				Warningf(warningMsg)
				Expect(logFileContains(logFile, warningMsg)).To(BeTrue())
				Infof(infoMsg)
				Expect(logFileContains(logFile, infoMsg)).To(BeTrue())
				Debugf(debugMsg)
				Expect(logFileContains(logFile, debugMsg)).To(BeFalse())
			})

			It("should print appropriate >= info structured messages to log file", func() {
				SetLogFile(logFileName)
				SetLogLevel(StringToLevel(infoStr))
				SetLogStderr(false)

				PanicStructured(panicMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, panicStr, panicMsg))).To(BeTrue())
				_ = ErrorStructured(errorMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, errorStr, errorMsg))).To(BeTrue())
				WarningStructured(warningMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, warningStr, warningMsg))).To(BeTrue())
				InfoStructured(infoMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, infoStr, infoMsg))).To(BeTrue())
				DebugStructured(debugMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, debugStr, debugMsg))).To(BeFalse())
			})
		})

		When("log level is set to DEBUG and messages are logged", func() {
			It("should print appropriate >= debug messages to log file", func() {
				SetLogFile(logFileName)
				SetLogLevel(StringToLevel(debugStr))
				SetLogStderr(false)

				Panicf(panicMsg)
				Expect(logFileContains(logFile, panicMsg)).To(BeTrue())
				_ = Errorf(errorMsg)
				Expect(logFileContains(logFile, errorMsg)).To(BeTrue())
				Warningf(warningMsg)
				Expect(logFileContains(logFile, warningMsg)).To(BeTrue())
				Infof(infoMsg)
				Expect(logFileContains(logFile, infoMsg)).To(BeTrue())
				Debugf(debugMsg)
				Expect(logFileContains(logFile, debugMsg)).To(BeTrue())
			})

			It("should print appropriate >= debug structured messages to log file", func() {
				SetLogFile(logFileName)
				SetLogLevel(StringToLevel(debugStr))
				SetLogStderr(false)

				PanicStructured(panicMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, panicStr, panicMsg))).To(BeTrue())
				_ = ErrorStructured(errorMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, errorStr, errorMsg))).To(BeTrue())
				WarningStructured(warningMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, warningStr, warningMsg))).To(BeTrue())
				InfoStructured(infoMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, infoStr, infoMsg))).To(BeTrue())
				DebugStructured(debugMsg)
				Expect(logFileContainsRegex(logFile, fmt.Sprintf(`time=".*" level=%q msg=%q`, debugStr, debugMsg))).To(BeTrue())
			})
		})

		When("stucturedMessage is called with an odd number of arguments", func() {
			It("should panic", func() {
				Expect(func() { structuredMessage(InfoLevel, infoMsg, "a", "b", "c") }).Should(PanicWith(MatchRegexp( //nolint:staticcheck
					fmt.Sprintf(`^time=".*" msg=%q logging_failure=%q$`, infoMsg, structuredLoggingOddArguments))))
			})
		})

		When("custom io.Writer is set", func() {
			var out bytes.Buffer

			BeforeEach(func() {
				out = bytes.Buffer{}
				SetOutput(&out)
				SetLogStderr(false)
			})

			It("should log message to custom out", func() {
				Infof(infoMsg)
				Expect(out.String()).To(ContainSubstring(infoMsg))
			})

			It("should not log to custom out after a call to SetLogFile", func() {
				SetLogFile(logFileName)
				Infof(infoMsg)
				Expect(out.String()).NotTo(ContainSubstring(infoMsg))
			})

			It("should not log to custom out after a call to SetLogOptions", func() {
				SetLogOptions(nil)
				Infof(infoMsg)
				Expect(out.String()).NotTo(ContainSubstring(infoMsg))
			})
		})

		When("error logging is on and file logging is off", func() {
			BeforeEach(func() {
				errStr := captureStdErr(SetLogStderr, true)
				Expect(errStr).To(BeEmpty())
				errStr = captureStdErr(SetLogFile, "")
				Expect(errStr).To(BeEmpty())
			})

			It("only logs to stderr", func() {
				errStr := captureStdErrEvent(Warningf, infoMsg)
				Expect(errStr).To(ContainSubstring(infoMsg))
				Expect(logFileContains(logFile, infoMsg)).To(BeFalse())
			})
		})

		When("file logging is on and error logging is off", func() {
			BeforeEach(func() {
				errStr := captureStdErr(SetLogFile, logFileName)
				Expect(errStr).To(BeEmpty())
				errStr = captureStdErr(SetLogStderr, false)
				Expect(errStr).To(BeEmpty())
			})

			It("only logs to file", func() {
				errStr := captureStdErrEvent(Infof, infoMsg)
				Expect(errStr).To(BeEmpty())
				Expect(logFileContains(logFile, infoMsg)).To(BeTrue())
			})
		})

		When("file logging and error logging are turned off simultaneously", func() {
			BeforeEach(func() {
				_ = captureStdErr(SetLogFile, "")
				_ = captureStdErr(SetLogStderr, false)
			})

			It("does not log anywhere", func() {
				errStr := captureStdErrEvent(Infof, infoMsg)
				Expect(errStr).To(BeEmpty())
				Expect(logFileContains(logFile, infoMsg)).To(BeFalse())
			})
		})

		When("file logging and error logging are turned on simultaneously", func() {
			BeforeEach(func() {
				Expect(captureStdErr(SetLogFile, logFileName)).To(BeEmpty())
				Expect(captureStdErr(SetLogStderr, true)).To(BeEmpty())
			})

			It("does log to both file and stderr", func() {
				errStr := captureStdErrEvent(Infof, infoMsg)
				Expect(errStr).To(ContainSubstring(infoMsg))
				Expect(logFileContains(logFile, infoMsg)).To(BeTrue())
			})
		})
	})

	Context("Updating the logging prefix", Ordered, func() {
		BeforeEach(func() {
			SetLogStderr(true)
			SetLogFile(logFileName)
		})

		When("a custom prefix is not provided", func() {
			It("uses the default prefix", func() {
				expectedPrefix := fmt.Sprintf(`^.* \[%s\] `, InfoLevel)
				errStr := captureStdErrEvent(Infof, infoMsg)
				Expect(errStr).To(MatchRegexp(expectedPrefix))
				Expect(logFileContainsRegex(logFile, expectedPrefix)).To(BeTrue())
			})
		})

		When("a custom prefix is provided", func() {
			BeforeEach(func() {
				SetLogLevel(StringToLevel(debugStr))
				SetPrefixer(&customPrefix{
					prefixFormat: "[%s/%s] - %s: ",
					currentFile:  "logging_test.go",
				})
			})

			It("uses the custom prefix", func() {
				expectedPrefix := "[info/debug] - logging_test.go: "
				errStr := captureStdErrEvent(Infof, infoMsg)
				Expect(errStr).To(ContainSubstring(expectedPrefix))
				Expect(logFileContains(logFile, expectedPrefix)).To(BeTrue())
			})

			It("uses the default prefix when explicitly requesting to do so", func() {
				SetDefaultPrefixer()

				expectedPrefix := fmt.Sprintf(`^.* \[%s\] `, InfoLevel)
				errStr := captureStdErrEvent(Infof, infoMsg)
				Expect(errStr).To(MatchRegexp(expectedPrefix))
				Expect(logFileContainsRegex(logFile, expectedPrefix)).To(BeTrue())
			})
		})
	})

	Context("Updating the structured logging prefix", Ordered, func() {
		BeforeEach(func() {
			SetLogStderr(true)
			SetLogFile(logFileName)
		})

		When("a custom structured prefix is not provided", func() {
			It("uses the default prefix", func() {
				expected := fmt.Sprintf(`time=".*" level=%q msg=%q`, infoStr, infoMsg)
				errStr := captureStdErrEvent(InfoStructured, infoMsg)
				Expect(errStr).To(MatchRegexp(expected))
				Expect(logFileContainsRegex(logFile, expected)).To(BeTrue())
			})
		})

		When("a custom structured prefix is provided", func() {
			BeforeEach(func() {
				SetLogLevel(StringToLevel(debugStr))
				SetStructuredPrefixer(&customPrefix{
					currentFile: "logging_test.go",
				})
			})

			It("uses the custom structured prefix", func() {
				expected := fmt.Sprintf(`custom-level=%q custom-file="logging_test.go" custom-message=%q`, infoStr, infoMsg)
				errStr := captureStdErrEvent(InfoStructured, infoMsg)
				Expect(errStr).To(MatchRegexp(expected))
				Expect(logFileContainsRegex(logFile, expected)).To(BeTrue())
			})

			It("uses the default structured prefix when explicitly requesting to do so", func() {
				SetDefaultStructuredPrefixer()

				expected := fmt.Sprintf(`time=".*" level=%q msg=%q`, infoStr, infoMsg)
				errStr := captureStdErrEvent(InfoStructured, infoMsg)
				Expect(errStr).To(MatchRegexp(expected))
				Expect(logFileContainsRegex(logFile, expected)).To(BeTrue())
			})
		})

		When("an invalid custom structured prefix is provided", func() {
			It("should panic", func() {
				var invalidPrefix StructuredPrefixerFunc = func(loggingLevel Level, message string) []interface{} {
					return []interface{}{
						"custom-level", loggingLevel,
						"custom-message", message,
						"invalid",
					}
				}
				SetStructuredPrefixer(invalidPrefix)

				Expect(func() { structuredMessage(InfoLevel, infoMsg, "a", "b", "c") }).Should(PanicWith(MatchRegexp( //nolint:staticcheck
					fmt.Sprintf(`^msg=%q logging_failure=%q$`, infoMsg, structuredPrefixerOddArguments))))
			})
		})

	})
})

var _ = Describe("CNI Log Path Security", func() {
	var baseDir string

	BeforeEach(func() {
		initLogger()
		baseDir = GinkgoT().TempDir()
		SetLogFileBaseDir(baseDir)
	})

	Context("SetLogFile path restrictions", func() {
		AfterEach(func() {
			disableFileLogging()
		})

		When("SetLogFile is called with a path containing traversal sequences", func() {
			It("should not create files via path traversal", func() {
				escapePath := "subdir/../../traversal-proof.log"

				errStr := captureStdErr(SetLogFile, escapePath)

				expectedLoggerOutput := fmt.Sprintf(logFileFailMsgWithError, escapePath, baseDirectoryEscapeFailMsg)
				Expect(errStr).To(Equal(expectedLoggerOutput))
				traversalTarget := filepath.Join(filepath.Dir(baseDir), "traversal-proof.log")
				Expect(traversalTarget).NotTo(BeAnExistingFile(),
					"VULNERABILITY: SetLogFile should reject paths with traversal sequences")
			})
		})

		When("SetLogFile is called with an arbitrary absolute path", func() {
			It("should not create files at unrestricted locations", func() {
				arbitraryDir := GinkgoT().TempDir()
				arbitraryPath := path.Join(arbitraryDir, "unrestricted.log")

				SetLogFile(arbitraryPath)

				Expect(arbitraryPath).NotTo(BeAnExistingFile(),
					"VULNERABILITY: SetLogFile should not create files at unrestricted paths without a configured base directory")
			})
		})

		When("SetLogFile is called with a path that creates nested directories", func() {
			It("should not create arbitrary directory trees", func() {
				arbitraryDir := GinkgoT().TempDir()
				deepPath := path.Join(arbitraryDir, "a", "b", "c", "evil.log")

				SetLogFile(deepPath)

				Expect(path.Join(arbitraryDir, "a")).NotTo(BeADirectory(),
					"VULNERABILITY: SetLogFile should not create arbitrary directory trees")
			})
		})

		When("SetLogFile is called with a relative traversal path", func() {
			It("should reject paths that escape the base directory", func() {
				escapePath := path.Join("..", "escape.log")
				errStr := captureStdErr(SetLogFile, escapePath)

				expectedLoggerOutput := fmt.Sprintf(logFileFailMsgWithError, escapePath, baseDirectoryEscapeFailMsg)
				Expect(errStr).To(Equal(expectedLoggerOutput))
				parentFile := filepath.Join(filepath.Dir(baseDir), "escape.log")
				Expect(parentFile).NotTo(BeAnExistingFile())
			})
		})

		When("a parent directory in the path is a symbolic link", func() {
			It("should reject the path", func() {
				outsideDir := GinkgoT().TempDir()
				linkDir := filepath.Join(baseDir, "linkdir")
				Expect(os.Symlink(outsideDir, linkDir)).To(Succeed())

				logPath := filepath.Join("linkdir", "app.log")
				loggerOutput := captureStdErr(SetLogFile, logPath)

				expectedLoggerOutput := fmt.Sprintf(logFileFailMsgWithError, logPath, symlinkEvalFailMsg)
				Expect(loggerOutput).To(Equal(expectedLoggerOutput))
				Expect(filepath.Join(outsideDir, "app.log")).NotTo(BeAnExistingFile())
			})
		})

		When("SetLogFile is called with a path that resolves to the base directory", func() {
			It("should reject the path", func() {
				errStr := captureStdErr(SetLogFile, ".")

				expectedLoggerOutput := fmt.Sprintf(logFileFailMsgWithError, ".", baseDirAsFileFailMsg)
				Expect(errStr).To(Equal(expectedLoggerOutput))
			})
		})

		When("SetLogFile is called with a valid relative path", func() {
			It("should create the file under the base directory", func() {
				SetLogFile("app.log")

				expectedPath := filepath.Join(baseDir, "app.log")
				Expect(expectedPath).To(BeAnExistingFile())
				Expect(logger.Filename).To(Equal(expectedPath))
			})
		})

		When("SetLogFile is called with a valid relative subdirectory path", func() {
			It("should create the file and directories under the base directory", func() {
				SetLogFile(path.Join("subdir", "app.log"))

				expectedPath := filepath.Join(baseDir, "subdir", "app.log")
				Expect(expectedPath).To(BeAnExistingFile())
				Expect(logger.Filename).To(Equal(expectedPath))
			})
		})

		When("SetLogFileBaseDir is used to override the default", func() {
			It("should resolve paths relative to the custom base directory", func() {
				customBase := GinkgoT().TempDir()
				SetLogFileBaseDir(customBase)

				SetLogFile("custom.log")

				expectedPath := filepath.Join(customBase, "custom.log")
				Expect(expectedPath).To(BeAnExistingFile())
				Expect(logger.Filename).To(Equal(expectedPath))
			})
		})

		When("SetLogFileBaseDir is called with an empty or current-directory path", func() {
			It("should reset to the default base directory for empty input", func() {
				SetLogFileBaseDir("")
				Expect(logFileBaseDir).To(Equal(defaultLogFileBaseDir))
			})

			It("should reset to the default base directory for current directory", func() {
				SetLogFileBaseDir(".")
				Expect(logFileBaseDir).To(Equal(defaultLogFileBaseDir))
			})
		})
	})
})

var _ = Describe("CNI Log Level Operations", func() {
	BeforeEach(func() {
		initLogger()
	})

	Context("Log level", func() {
		Context("Converting strings to Levels", func() {
			When("a valid string is passed", func() {
				It("returns the correct level value", func() {
					Expect(StringToLevel(warningStr)).To(Equal(WarningLevel))
					Expect(StringToLevel("ERROR")).To(Equal(ErrorLevel))
					Expect(StringToLevel("dEbUg")).To(Equal(DebugLevel))
				})
			})

			When("an invalid string is passed", func() {
				It("returns InvalidLevel (-1)", func() {
					Expect(StringToLevel(invalidStr)).To(Equal(InvalidLevel))
				})
			})
		})

		Context("Setting the log level", func() {
			When("a valid log level argument is passed in", func() {
				It("sets the appropriate log level", func() {
					// by string
					SetLogLevel(StringToLevel(debugStr))
					Expect(logLevel).To(Equal(DebugLevel))
					SetLogLevel(StringToLevel(infoStr))
					Expect(logLevel).To(Equal(InfoLevel))
					SetLogLevel(StringToLevel(warningStr))
					Expect(logLevel).To(Equal(WarningLevel))
					SetLogLevel(StringToLevel(errorStr))
					Expect(logLevel).To(Equal(ErrorLevel))
					SetLogLevel(StringToLevel(panicStr))
					Expect(logLevel).To(Equal(PanicLevel))
					// by int
					for i := 1; i <= 5; i++ {
						l := Level(i)
						SetLogLevel(l)
						Expect(logLevel).To(Equal(l))
					}
					// by level
					SetLogLevel(DebugLevel)
					Expect(logLevel).To(Equal(DebugLevel))
					SetLogLevel(WarningLevel)
					Expect(logLevel).To(Equal(WarningLevel))
				})
			})

			When("an invalid log level argument is passed in", func() {
				invalidLogLevel := Level(-1)
				It("maintains the current log level and logs an error", func() {
					expectedLoggerOutput := fmt.Sprintf(setLevelFailMsg, invalidLogLevel)
					loggerOutput := captureStdErr(SetLogLevel, invalidLogLevel)

					Expect(loggerOutput).To(Equal(expectedLoggerOutput))
					Expect(logLevel).To(Equal(defaultLogLevel))

					invalidLogLevel = Level(10)
					expectedLoggerOutput = fmt.Sprintf(setLevelFailMsg, invalidLogLevel)
					loggerOutput = captureStdErr(SetLogLevel, invalidLogLevel)

					Expect(loggerOutput).To(Equal(expectedLoggerOutput))
					Expect(logLevel).To(Equal(defaultLogLevel))
				})
			})
		})
	})

})

// Checks if the message was logged to the log file.
func logFileContains(filename, subString string) bool {
	// Read in the log file
	contents, err := os.ReadFile(filename)
	if err != nil {
		return false
	}
	return strings.Contains(string(contents), subString)
}

// Checks if the message was logged to the log file by comparing to regular expression re.
func logFileContainsRegex(filename, re string) bool {
	// Read in the log file
	contents, err := os.ReadFile(filename)
	if err != nil {
		return false
	}
	matched, err := regexp.MatchString(re, string(contents))
	if err != nil {
		panic(err)
	}
	return matched
}

func openPipes() (*os.File, *os.File, *os.File) {
	origWriter := os.Stderr

	pipeReader, pipeWriter, err := os.Pipe() // Initialize an IO pipe
	if err != nil {
		panic(err)
	}

	os.Stderr = pipeWriter // Set stderr to point to the pipe's writer

	return pipeReader, pipeWriter, origWriter
}

func closePipes(reader, writer, orig *os.File) string {
	writer.Close()
	os.Stderr = orig // Revert stderr to what it used to be

	var buff bytes.Buffer
	_, err := io.Copy(&buff, reader) // populate a buffer with data passed in through the pipe
	if err != nil {
		panic(err) // If error is not nil then panics
	}

	return buff.String()
}

func captureStdErr[T any](f func(T), p T) string {
	pipeWriter, pipeReader, origWriter := openPipes()
	f(p)
	return closePipes(pipeWriter, pipeReader, origWriter)
}

func captureStdErrEvent(f func(string, ...interface{}), s string, a ...interface{}) string { //nolint:unparam
	pipeWriter, pipeReader, origWriter := openPipes()
	f(s, a...)
	return closePipes(pipeWriter, pipeReader, origWriter)
}

func getPrimitivePointer[P int | bool](param P) *P {
	return &param
}
