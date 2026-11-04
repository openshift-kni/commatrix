package errhandler

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Errhandler", func() {
	var (
		origExit   func(int)
		origStderr io.Writer
		exitCode   int
		exitCalled bool
		buf        bytes.Buffer
	)

	BeforeEach(func() {
		origExit = osExit
		origStderr = stderr
		exitCode = 0
		exitCalled = false
		buf.Reset()

		osExit = func(code int) {
			exitCode = code
			exitCalled = true
		}
		stderr = &buf
	})

	AfterEach(func() {
		osExit = origExit
		stderr = origStderr
	})

	Describe("HandleAndExit", func() {
		It("does nothing when error is nil", func() {
			HandleAndExit(nil)
			Expect(exitCalled).To(BeFalse())
			Expect(buf.Len()).To(BeZero())
		})

		It("prints the error and exits with code 1", func() {
			HandleAndExit(errors.New("something broke"))
			Expect(exitCalled).To(BeTrue())
			Expect(exitCode).To(Equal(1))
			Expect(buf.String()).To(Equal("error: something broke\n"))
		})

		It("prints the full wrapped error chain", func() {
			inner := errors.New("root cause")
			wrapped := fmt.Errorf("outer context: %w", inner)
			HandleAndExit(wrapped)
			Expect(buf.String()).To(Equal("error: outer context: root cause\n"))
		})
	})

	Describe("RecoverAndExit", func() {
		It("does nothing when there is no panic", func() {
			func() {
				defer RecoverAndExit()
			}()
			Expect(exitCalled).To(BeFalse())
			Expect(buf.Len()).To(BeZero())
		})

		It("recovers an error panic and exits", func() {
			func() {
				defer RecoverAndExit()
				panic(errors.New("panic error"))
			}()
			Expect(exitCalled).To(BeTrue())
			Expect(exitCode).To(Equal(1))
			Expect(buf.String()).To(Equal("error: panic error\n"))
		})

		It("recovers a string panic and wraps it as an error", func() {
			func() {
				defer RecoverAndExit()
				panic("string panic")
			}()
			Expect(exitCalled).To(BeTrue())
			Expect(buf.String()).To(Equal("error: string panic\n"))
		})

		It("recovers a non-error, non-string panic", func() {
			func() {
				defer RecoverAndExit()
				panic(42)
			}()
			Expect(exitCalled).To(BeTrue())
			Expect(buf.String()).To(Equal("error: 42\n"))
		})
	})
})
