import { useState, type FormEvent } from "react"

import { ApiError, emailDocument } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

// Inline panel to email a printable document to its counterparty as a PDF
// attachment — the same PDF the download button produces. Recipients are typed
// here because organizations don't yet carry an email address (a documented
// follow-up); once they do, a blank field can fall back to the counterparty's
// address. Sending is inert unless SMTP is configured: the demo returns 501
// "email sending is not configured", surfaced here as the error.
export function EmailDocumentPanel({
  collection,
  documentId,
  label,
  onClose,
}: {
  /** API path segment shared with the PDF endpoint, e.g. "sales-invoices". */
  collection: string
  documentId: number
  /** Human document name for the confirmation copy, e.g. "Invoice". */
  label: string
  onClose: () => void
}) {
  const [to, setTo] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [sentTo, setSentTo] = useState<string[] | null>(null)

  function submit(e: FormEvent) {
    e.preventDefault()
    const recipients = to
      .split(",")
      .map((s) => s.trim())
      .filter((s) => s !== "")
    if (recipients.length === 0) {
      setError("Enter at least one recipient email address.")
      return
    }
    setBusy(true)
    setError(null)
    emailDocument(collection, documentId, recipients)
      .then(() => {
        setBusy(false)
        setSentTo(recipients)
      })
      .catch((err: unknown) => {
        setBusy(false)
        setError(err instanceof ApiError ? err.message : String(err))
      })
  }

  return (
    <form
      onSubmit={submit}
      className="mb-6 space-y-4 rounded-md border p-4"
      aria-label="Email"
    >
      {sentTo !== null ? (
        <>
          <p className="text-sm">
            Emailed {label.toLowerCase()} to {sentTo.join(", ")}.
          </p>
          <Button type="button" variant="outline" onClick={onClose}>
            Done
          </Button>
        </>
      ) : (
        <>
          <p className="text-sm text-muted-foreground">
            Emails this {label.toLowerCase()} as a PDF attachment.
          </p>
          <div className="space-y-2">
            <Label htmlFor="email_to">To</Label>
            <Input
              id="email_to"
              type="text"
              placeholder="name@example.com, another@example.com"
              value={to}
              onChange={(e) => setTo(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Comma-separate multiple recipients.
            </p>
          </div>
          {error !== null && (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          )}
          <div className="flex gap-2">
            <Button type="submit" disabled={busy}>
              {busy ? "Sending…" : "Send"}
            </Button>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
          </div>
        </>
      )}
    </form>
  )
}
