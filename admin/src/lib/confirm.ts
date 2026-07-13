/**
 * A small sweetalert2 wrapper for destructive-action confirmations, styled to
 * match the theme's buttons (see views/admin/plugins/sweet-alerts). Returns a
 * promise that resolves true only when the user confirms — callers gate the
 * irreversible request on it.
 */

import Swal from 'sweetalert2'

export type ConfirmOptions = {
  title: string
  text: string
  confirmText?: string
  cancelText?: string
  /** Style the confirm button as a danger action (destructive). Default true. */
  danger?: boolean
}

export async function confirmAction(options: ConfirmOptions): Promise<boolean> {
  const danger = options.danger ?? true
  const result = await Swal.fire({
    title: options.title,
    text: options.text,
    icon: 'warning',
    showCancelButton: true,
    focusCancel: true,
    buttonsStyling: false,
    confirmButtonText: options.confirmText ?? 'Confirm',
    cancelButtonText: options.cancelText ?? 'Cancel',
    // Skip the exit animation: it keeps the fire() promise resolving promptly
    // (jsdom emits no transitionend, which would otherwise stall it in tests)
    // and removing the fade-out is imperceptible in the browser.
    hideClass: { popup: '' },
    customClass: {
      confirmButton: `btn ${danger ? 'bg-danger hover:bg-danger-hover' : 'bg-primary hover:bg-primary-hover'} text-white me-2`,
      cancelButton: 'btn bg-light hover:text-primary',
    },
  })
  return result.isConfirmed === true
}
