import { cellRefToRowCol } from './cell-utils'

type CellGetter = (row: number, col: number) => string | undefined

interface FormulaResult {
    value: string
    error?: string
}

const CELL_REF_PATTERN = /\b([A-Z]+)(\d+)\b/g
const RANGE_PATTERN = /\b([A-Z]+\d+):([A-Z]+\d+)\b/g

function parseRange(start: string, end: string): { row: number; col: number }[] {
    const s = cellRefToRowCol(start)
    const e = cellRefToRowCol(end)
    const cells: { row: number; col: number }[] = []
    const minRow = Math.min(s.row, e.row)
    const maxRow = Math.max(s.row, e.row)
    const minCol = Math.min(s.col, e.col)
    const maxCol = Math.max(s.col, e.col)
    for (let r = minRow; r <= maxRow; r++) {
        for (let c = minCol; c <= maxCol; c++) {
            cells.push({ row: r, col: c })
        }
    }
    return cells
}

function resolveValues(cells: { row: number; col: number }[], getCellValue: CellGetter): number[] {
    const values: number[] = []
    for (const { row, col } of cells) {
        const raw = getCellValue(row, col)
        if (raw !== undefined && raw !== '') {
            const n = Number(raw)
            if (!Number.isNaN(n)) values.push(n)
        }
    }
    return values
}

function parseCellArgs(args: string[]): { row: number; col: number }[] {
    const result: { row: number; col: number }[] = []
    for (const arg of args) {
        const trimmed = arg.trim()
        const rangeMatch = trimmed.match(/^([A-Z]+\d+):([A-Z]+\d+)$/)
        if (rangeMatch) {
            result.push(...parseRange(rangeMatch[1], rangeMatch[2]))
            continue
        }
        const cellMatch = trimmed.match(/^([A-Z]+)(\d+)$/)
        if (cellMatch) {
            result.push(cellRefToRowCol(trimmed))
        }
    }
    return result
}

function evaluateAggregate(fn: string, values: number[]): string {
    switch (fn) {
        case 'SUM':
            return values.reduce((a, b) => a + b, 0).toString()
        case 'AVERAGE':
        case 'AVG':
            if (values.length === 0) return '#DIV/0!'
            return (values.reduce((a, b) => a + b, 0) / values.length).toString()
        case 'COUNT':
            return values.length.toString()
        case 'MIN':
            return values.length === 0 ? '0' : Math.min(...values).toString()
        case 'MAX':
            return values.length === 0 ? '0' : Math.max(...values).toString()
        default:
            return '#NAME?'
    }
}

function evaluateFunction(name: string, args: string[], getCellValue: CellGetter): string {
    const fn = name.toUpperCase()

    if (fn === 'IF') {
        if (args.length < 3) return '#VALUE!'
        const condResult = evaluateExpression(args[0].trim(), getCellValue)
        const isTruthy =
            condResult !== '0' &&
            condResult !== 'false' &&
            condResult !== '' &&
            !condResult.startsWith('#')
        return evaluateExpression(isTruthy ? args[1].trim() : args[2].trim(), getCellValue)
    }

    const allCells = parseCellArgs(args)
    const values = resolveValues(allCells, getCellValue)
    return evaluateAggregate(fn, values)
}

function evaluateExpression(expr: string, getCellValue: CellGetter): string {
    const trimmed = expr.trim()

    // String literal
    if (trimmed.startsWith('"') && trimmed.endsWith('"')) {
        return trimmed.slice(1, -1)
    }

    // Function call
    const funcMatch = trimmed.match(/^([A-Z]+)\(([\s\S]*)\)$/)
    if (funcMatch) {
        const funcName = funcMatch[1]
        const argsStr = funcMatch[2]
        const args = splitArgs(argsStr)
        return evaluateFunction(funcName, args, getCellValue)
    }

    // Replace cell references with their values for arithmetic
    const resolved = trimmed.replace(CELL_REF_PATTERN, match => {
        const { row, col } = cellRefToRowCol(match)
        const val = getCellValue(row, col)
        if (val === undefined || val === '') return '0'
        const n = Number(val)
        return Number.isNaN(n) ? '0' : n.toString()
    })

    // Simple arithmetic evaluation (supports +, -, *, /)
    try {
        const result = safeEval(resolved)
        if (result === null) return trimmed
        return result.toString()
    } catch {
        return trimmed
    }
}

function splitArgs(s: string): string[] {
    const args: string[] = []
    let depth = 0
    let current = ''
    for (const ch of s) {
        if (ch === '(') depth++
        else if (ch === ')') depth--
        if (ch === ',' && depth === 0) {
            args.push(current)
            current = ''
        } else {
            current += ch
        }
    }
    if (current) args.push(current)
    return args
}

function safeEval(expr: string): number | null {
    // Only allow numbers, operators, parentheses, spaces, and decimals
    if (!/^[\d\s+\-*/().]+$/.test(expr)) return null
    try {
        const result = Function(`"use strict"; return (${expr})`)()
        if (typeof result !== 'number' || !Number.isFinite(result)) return null
        return Math.round(result * 1e10) / 1e10
    } catch {
        return null
    }
}

export function evaluateFormula(formula: string, getCellValue: CellGetter): FormulaResult {
    if (!formula.startsWith('=')) {
        return { value: formula }
    }

    const expr = formula.slice(1).trim()
    if (!expr) return { value: '' }

    try {
        const result = evaluateExpression(expr, getCellValue)
        if (result.startsWith('#')) {
            return { value: result, error: result }
        }
        return { value: result }
    } catch {
        return { value: '#ERROR!', error: '#ERROR!' }
    }
}

export function getDependencies(formula: string): { row: number; col: number }[] {
    if (!formula.startsWith('=')) return []
    const expr = formula.slice(1)
    const deps: { row: number; col: number }[] = []

    // Extract ranges
    const rangeMatches = Array.from(expr.matchAll(new RegExp(RANGE_PATTERN.source, 'g')))
    for (const m of rangeMatches) {
        deps.push(...parseRange(m[1], m[2]))
    }

    // Extract individual cell refs (excluding those already in ranges)
    const withoutRanges = expr.replace(RANGE_PATTERN, '')
    const cellMatches = Array.from(withoutRanges.matchAll(new RegExp(CELL_REF_PATTERN.source, 'g')))
    for (const m of cellMatches) {
        deps.push(cellRefToRowCol(m[0]))
    }

    return deps
}

export function detectCircular(
    row: number,
    col: number,
    formula: string,
    getFormula: (r: number, c: number) => string | undefined,
    visited = new Set<string>()
): boolean {
    const key = `${row}:${col}`
    if (visited.has(key)) return true
    visited.add(key)

    const deps = getDependencies(formula)
    for (const dep of deps) {
        const depFormula = getFormula(dep.row, dep.col)
        if (!depFormula) continue
        if (detectCircular(dep.row, dep.col, depFormula, getFormula, visited)) {
            return true
        }
    }

    visited.delete(key)
    return false
}
