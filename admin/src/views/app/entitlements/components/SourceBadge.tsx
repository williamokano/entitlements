// A chip showing which resolution layer an effective value won from
// (default < plan < addon < override). The colour makes a manual `override`
// stand out from a catalog-derived plan/addon value.
import type { Source } from '../api'
import { SOURCE_LABELS, sourceBadgeClass } from '../helpers'

const SourceBadge = ({ source }: { source: Source }) => (
  <span className={`badge badge-label ${sourceBadgeClass(source)}`}>{SOURCE_LABELS[source]}</span>
)

export default SourceBadge
