// Renders an effective entitlement value by its inferred kind: a boolean shows
// an on/off chip, a numeric limit shows the number, and a config/enum value is
// shown verbatim (or compact JSON for a structured value). The feature type is
// not on the wire, so the kind is inferred from the value's JSON type.
import type { EntitlementValue } from '../api'
import { formatValue, valueKind } from '../helpers'

const ValueCell = ({ value }: { value: EntitlementValue }) => {
  const kind = valueKind(value)

  if (kind === 'boolean') {
    const on = value === true
    return (
      <span className={`badge badge-label ${on ? 'bg-success/15 text-success' : 'bg-default-200 text-default-600'}`}>
        {on ? 'On' : 'Off'}
      </span>
    )
  }

  if (kind === 'limit') {
    return <span className="font-medium tabular-nums">{formatValue(value)}</span>
  }

  // config / enum — render the value verbatim (compact JSON if structured).
  return <code className="text-default-600 rounded bg-default-100 px-1.5 py-0.5 text-xs">{formatValue(value)}</code>
}

export default ValueCell
