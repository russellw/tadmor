import { useCallback, useEffect, useState, type FormEvent } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import { formatAmount, isZeroAmount } from "@/lib/amount"
import {
  ApiError,
  addBankStatementLine,
  autoMatchBankStatement,
  deleteBankStatement,
  deleteBankStatementLine,
  getBankMatchCandidates,
  getBankStatement,
  getBankStatementLines,
  importBankStatementCSV,
  matchBankStatementLine,
  reconcileBankStatement,
  reopenBankStatement,
  unmatchBankStatementLine,
  type BankMatchCandidate,
  type BankStatement,
  type BankStatementLine,
} from "@/lib/api"
import { today } from "@/lib/dates"
import { useCurrentUser } from "@/lib/current-user"
import { AmountCell } from "@/components/amount-cell"
import { ReconciledBadge } from "@/components/bank-statements"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// The reconciliation workspace: one statement's transactions on the left of
// each row, its match into the ledger on the right. Transactions are captured
// by CSV import or keyed one at a time; Auto-match pairs lines with posted
// journal lines of the same amount (nearest date first); the picker under an
// unmatched line resolves the rest by hand. Reconcile locks the statement
// once every line is matched and opening + lines = closing.
export function BankStatementDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const statementId = Number(id)
  const currentUser = useCurrentUser()

  const [statement, setStatement] = useState<BankStatement | null>(null)
  const [lines, setLines] = useState<BankStatementLine[] | null>(null)
  const [candidates, setCandidates] = useState<BankMatchCandidate[]>([])
  const [error, setError] = useState<string | null>(null)
  const [acting, setActing] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  // Which unmatched line has its candidate picker open, and whether the
  // picker is limited to candidates of the line's amount (the default).
  const [pickingLineId, setPickingLineId] = useState<number | null>(null)
  const [showAllCandidates, setShowAllCandidates] = useState(false)
  const [newLine, setNewLine] = useState({
    txnDate: today(),
    description: "",
    reference: "",
    amount: "",
  })
  const [csvText, setCsvText] = useState("")
  const [showImport, setShowImport] = useState(false)

  const load = useCallback(async () => {
    const [s, ls, cands] = await Promise.all([
      getBankStatement(statementId),
      getBankStatementLines(statementId),
      getBankMatchCandidates(statementId),
    ])
    return { s, ls, cands }
  }, [statementId])

  const applyLoaded = useCallback(
    ({
      s,
      ls,
      cands,
    }: {
      s: BankStatement
      ls: BankStatementLine[]
      cands: BankMatchCandidate[]
    }) => {
      setStatement(s)
      setLines(ls)
      setCandidates(cands)
    },
    [],
  )

  useEffect(() => {
    if (!Number.isInteger(statementId) || statementId <= 0) {
      setError("Invalid id.")
      return
    }
    let cancelled = false
    load()
      .then((data) => {
        if (!cancelled) applyLoaded(data)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [statementId, load, applyLoaded])

  function runAction(action: () => Promise<unknown>) {
    setActing(true)
    setActionError(null)
    action()
      .then(load)
      .then((data) => {
        applyLoaded(data)
        setActing(false)
        setPickingLineId(null)
        setShowAllCandidates(false)
      })
      .catch((err: unknown) => {
        setActing(false)
        setActionError(err instanceof ApiError ? err.message : String(err))
      })
  }

  function runDelete() {
    if (
      !window.confirm(
        "Delete this statement and all of its lines? This cannot be undone.",
      )
    ) {
      return
    }
    setActing(true)
    setActionError(null)
    deleteBankStatement(statementId)
      .then(() => navigate("/bank-statements"))
      .catch((err: unknown) => {
        setActing(false)
        setActionError(err instanceof ApiError ? err.message : String(err))
      })
  }

  function handleAddLine(e: FormEvent) {
    e.preventDefault()
    runAction(() =>
      addBankStatementLine(statementId, {
        txn_date: newLine.txnDate,
        description: newLine.description.trim(),
        reference:
          newLine.reference.trim() === "" ? null : newLine.reference.trim(),
        amount: newLine.amount,
      }).then(() =>
        setNewLine({
          txnDate: newLine.txnDate,
          description: "",
          reference: "",
          amount: "",
        }),
      ),
    )
  }

  function handleImport() {
    runAction(() =>
      importBankStatementCSV(statementId, csvText).then(() => {
        setCsvText("")
        setShowImport(false)
      }),
    )
  }

  if (error !== null) {
    return (
      <section className="mx-auto w-full max-w-6xl space-y-4 p-6">
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
        <Button variant="outline" asChild>
          <Link to="/bank-statements">Back</Link>
        </Button>
      </section>
    )
  }
  if (statement === null || lines === null) {
    return (
      <section className="mx-auto w-full max-w-6xl p-6">
        <p className="text-sm text-muted-foreground">Loading…</p>
      </section>
    )
  }

  const open = statement.status === "open"
  const balanced = isZeroAmount(statement.difference)
  const allMatched = statement.matched_count === statement.line_count

  return (
    <section className="mx-auto w-full max-w-6xl p-6">
      <header className="mb-6 flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-semibold tracking-tight">
              Bank Statement #{statement.id}
            </h1>
            <ReconciledBadge status={statement.status} />
          </div>
          <p className="text-sm text-muted-foreground">
            {statement.account_code} {statement.account_name} ·{" "}
            {statement.statement_date}
            {statement.reference !== null && ` · ${statement.reference}`}
          </p>
        </div>
        <div className="flex gap-2">
          {open && (
            <Button
              disabled={acting}
              onClick={() => runAction(() => autoMatchBankStatement(statementId))}
            >
              {acting ? "Working…" : "Auto-match"}
            </Button>
          )}
          {open && (
            <Button
              disabled={acting || !allMatched || !balanced}
              title={
                allMatched && balanced
                  ? undefined
                  : "Every line must be matched and the statement must add up."
              }
              onClick={() => runAction(() => reconcileBankStatement(statementId))}
            >
              Reconcile
            </Button>
          )}
          {open && (
            <Button variant="outline" disabled={acting} asChild>
              <Link to={`/bank-statements/${statementId}/edit`}>Edit</Link>
            </Button>
          )}
          {open && (
            <Button variant="outline" disabled={acting} onClick={runDelete}>
              Delete
            </Button>
          )}
          {!open && currentUser.is_admin && (
            <Button
              variant="outline"
              disabled={acting}
              onClick={() => runAction(() => reopenBankStatement(statementId))}
            >
              {acting ? "Reopening…" : "Reopen"}
            </Button>
          )}
          <Button variant="outline" asChild>
            <Link to="/bank-statements">Back</Link>
          </Button>
        </div>
      </header>

      <dl className="mb-6 grid grid-cols-2 gap-4 text-sm sm:grid-cols-5">
        {[
          ["Opening", statement.opening_balance],
          ["Lines", statement.lines_total],
          ["Closing", statement.closing_balance],
        ].map(([label, value]) => (
          <div key={label}>
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="font-mono tabular-nums">{formatAmount(value)}</dd>
          </div>
        ))}
        <div>
          <dt className="text-muted-foreground">Difference</dt>
          <dd
            className={`font-mono tabular-nums ${balanced ? "" : "text-destructive"}`}
          >
            {formatAmount(statement.difference)}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Matched</dt>
          <dd className="font-mono tabular-nums">
            {statement.matched_count}/{statement.line_count}
          </dd>
        </div>
      </dl>

      {actionError !== null && (
        <p className="mb-4 text-sm text-destructive" role="alert">
          {actionError}
        </p>
      )}

      {lines.length === 0 && (
        <p className="mb-4 text-sm text-muted-foreground">
          No transactions yet. Import the statement as CSV or add lines below.
        </p>
      )}

      {lines.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-12">#</TableHead>
              <TableHead>Date</TableHead>
              <TableHead>Description</TableHead>
              <TableHead>Reference</TableHead>
              <TableHead className="text-right">Amount</TableHead>
              <TableHead>Matched To</TableHead>
              {open && <TableHead className="w-40" />}
            </TableRow>
          </TableHeader>
          <TableBody>
            {lines.map((l) => {
              const picking = pickingLineId === l.id
              const lineCandidates = showAllCandidates
                ? candidates
                : candidates.filter((c) => c.amount === l.amount)
              return (
                <LineRows
                  key={l.id}
                  line={l}
                  open={open}
                  acting={acting}
                  picking={picking}
                  candidates={picking ? lineCandidates : []}
                  showAllCandidates={showAllCandidates}
                  onToggleShowAll={() => setShowAllCandidates((v) => !v)}
                  onTogglePicker={() => {
                    setPickingLineId(picking ? null : l.id)
                    setShowAllCandidates(false)
                  }}
                  onMatch={(journalLineId) =>
                    runAction(() => matchBankStatementLine(l.id, journalLineId))
                  }
                  onUnmatch={() =>
                    runAction(() => unmatchBankStatementLine(l.id))
                  }
                  onDelete={() =>
                    runAction(() => deleteBankStatementLine(l.id))
                  }
                />
              )
            })}
          </TableBody>
        </Table>
      )}

      {open && (
        <div className="mt-6 space-y-4">
          <form
            onSubmit={handleAddLine}
            className="flex flex-wrap items-end gap-2"
          >
            <div className="space-y-1">
              <label
                htmlFor="line_date"
                className="text-xs text-muted-foreground"
              >
                Date
              </label>
              <Input
                id="line_date"
                type="date"
                required
                className="w-40"
                value={newLine.txnDate}
                onChange={(e) =>
                  setNewLine({ ...newLine, txnDate: e.target.value })
                }
              />
            </div>
            <div className="min-w-48 flex-1 space-y-1">
              <label
                htmlFor="line_description"
                className="text-xs text-muted-foreground"
              >
                Description
              </label>
              <Input
                id="line_description"
                required
                value={newLine.description}
                onChange={(e) =>
                  setNewLine({ ...newLine, description: e.target.value })
                }
              />
            </div>
            <div className="space-y-1">
              <label
                htmlFor="line_reference"
                className="text-xs text-muted-foreground"
              >
                Reference
              </label>
              <Input
                id="line_reference"
                className="w-32"
                value={newLine.reference}
                onChange={(e) =>
                  setNewLine({ ...newLine, reference: e.target.value })
                }
              />
            </div>
            <div className="space-y-1">
              <label
                htmlFor="line_amount"
                className="text-xs text-muted-foreground"
              >
                Amount
              </label>
              <Input
                id="line_amount"
                inputMode="decimal"
                required
                placeholder="deposits +, payments −"
                className="w-40"
                value={newLine.amount}
                onChange={(e) =>
                  setNewLine({ ...newLine, amount: e.target.value })
                }
              />
            </div>
            <Button type="submit" variant="outline" disabled={acting}>
              Add line
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={acting}
              onClick={() => setShowImport((v) => !v)}
            >
              {showImport ? "Hide import" : "Import CSV…"}
            </Button>
          </form>

          {showImport && (
            <div className="space-y-2">
              <Textarea
                rows={6}
                placeholder={
                  "date,description,amount,reference\n2026-06-10,Incoming transfer,1250.00,REF-1\n2026-06-12,Card settlement,-84.50"
                }
                value={csvText}
                onChange={(e) => setCsvText(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Columns: date (YYYY-MM-DD), description, amount (deposits
                positive, payments negative), optional reference. A header row
                is skipped automatically.
              </p>
              <Button
                disabled={acting || csvText.trim() === ""}
                onClick={handleImport}
              >
                {acting ? "Importing…" : "Import"}
              </Button>
            </div>
          )}
        </div>
      )}
    </section>
  )
}

// One statement line, plus — when its candidate picker is open — a full-width
// row listing the journal lines it can be matched to.
function LineRows({
  line,
  open,
  acting,
  picking,
  candidates,
  showAllCandidates,
  onToggleShowAll,
  onTogglePicker,
  onMatch,
  onUnmatch,
  onDelete,
}: {
  line: BankStatementLine
  open: boolean
  acting: boolean
  picking: boolean
  candidates: BankMatchCandidate[]
  showAllCandidates: boolean
  onToggleShowAll: () => void
  onTogglePicker: () => void
  onMatch: (journalLineId: number) => void
  onUnmatch: () => void
  onDelete: () => void
}) {
  const matched = line.journal_line_id !== null
  return (
    <>
      <TableRow>
        <TableCell className="font-mono text-muted-foreground">
          {line.line_no}
        </TableCell>
        <TableCell className="text-muted-foreground">{line.txn_date}</TableCell>
        <TableCell className="font-medium">{line.description}</TableCell>
        <TableCell className="text-muted-foreground">
          {line.reference ?? "—"}
        </TableCell>
        <AmountCell value={line.amount} />
        <TableCell>
          {matched ? (
            <span className="text-sm">
              <Link
                to={`/journal-entries/${line.journal_entry_id}`}
                className="text-primary hover:underline"
              >
                entry #{line.journal_entry_id}
              </Link>{" "}
              <span className="text-muted-foreground">
                · {line.entry_date}
                {line.entry_memo !== null && ` · ${line.entry_memo}`}
              </span>
            </span>
          ) : (
            <span className="text-sm text-muted-foreground">unmatched</span>
          )}
        </TableCell>
        {open && (
          <TableCell className="space-x-2 text-right">
            {matched ? (
              <Button
                variant="outline"
                size="sm"
                disabled={acting}
                onClick={onUnmatch}
              >
                Unmatch
              </Button>
            ) : (
              <Button
                variant={picking ? "secondary" : "outline"}
                size="sm"
                disabled={acting}
                onClick={onTogglePicker}
              >
                {picking ? "Close" : "Match…"}
              </Button>
            )}
            <Button
              variant="outline"
              size="sm"
              disabled={acting}
              onClick={onDelete}
              aria-label={`Delete line ${line.line_no}`}
            >
              ✕
            </Button>
          </TableCell>
        )}
      </TableRow>
      {picking && (
        <TableRow className="bg-muted/30 hover:bg-muted/30">
          <TableCell colSpan={open ? 7 : 6} className="p-3">
            {candidates.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No unmatched ledger entries
                {showAllCandidates ? "" : " of this amount"} on this account.{" "}
                {!showAllCandidates && (
                  <button
                    type="button"
                    className="text-primary hover:underline"
                    onClick={onToggleShowAll}
                  >
                    Show all amounts
                  </button>
                )}
              </p>
            ) : (
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">
                  Unmatched ledger entries
                  {showAllCandidates ? "" : " of this amount"} ·{" "}
                  <button
                    type="button"
                    className="text-primary hover:underline"
                    onClick={onToggleShowAll}
                  >
                    {showAllCandidates ? "Only this amount" : "Show all amounts"}
                  </button>
                </p>
                <ul className="divide-y">
                  {candidates.map((c) => (
                    <li
                      key={c.journal_line_id}
                      className="flex items-center gap-3 py-1.5 text-sm"
                    >
                      <span className="text-muted-foreground">
                        {c.entry_date}
                      </span>
                      <Link
                        to={`/journal-entries/${c.journal_entry_id}`}
                        className="text-primary hover:underline"
                      >
                        entry #{c.journal_entry_id}
                      </Link>
                      <span className="min-w-0 flex-1 truncate">
                        {c.memo ?? c.reference ?? ""}
                      </span>
                      <span className="font-mono tabular-nums">
                        {formatAmount(c.amount)}
                      </span>
                      <Button
                        size="sm"
                        disabled={acting}
                        onClick={() => onMatch(c.journal_line_id)}
                      >
                        Match
                      </Button>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </TableCell>
        </TableRow>
      )}
    </>
  )
}
