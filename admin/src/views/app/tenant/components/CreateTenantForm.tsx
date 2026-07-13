// Create-tenant form, shared by the onboarding empty state and the
// "add another tenant" screen the switcher links to. On success the new tenant
// is remembered and made current (so subsequent requests carry its
// X-Tenant-ID), then the caller-supplied onCreated hook routes onward.
import { yupResolver } from '@hookform/resolvers/yup'
import { useState } from 'react'
import { type SubmitHandler, useForm } from 'react-hook-form'
import * as yup from 'yup'
import { ApiError } from '@/lib/api'
import { rememberAndSelect } from '@/lib/tenant'
import { cn } from '@/utils/helpers'
import { createTenant, type Tenant } from '../api'

type FormValues = {
  name: string
  slug: string
}

const slugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

const schema: yup.ObjectSchema<FormValues> = yup.object({
  name: yup.string().required('Enter a tenant name').max(120, 'Name is too long'),
  slug: yup
    .string()
    .required('Enter a slug')
    .matches(slugPattern, 'Use lowercase letters, numbers and single hyphens')
    .max(63, 'Slug is too long'),
})

const slugify = (value: string): string =>
  value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')

type Props = {
  /** Called with the created tenant after it has been stored and selected. */
  onCreated: (tenant: Tenant) => void
  submitLabel?: string
}

const CreateTenantForm = ({ onCreated, submitLabel = 'Create tenant' }: Props) => {
  const [error, setError] = useState<string | null>(null)
  const [slugEdited, setSlugEdited] = useState(false)

  const {
    register,
    handleSubmit,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ mode: 'onSubmit', resolver: yupResolver(schema) })

  const onSubmit: SubmitHandler<FormValues> = async ({ name, slug }) => {
    setError(null)
    try {
      const tenant = await createTenant({ name: name.trim(), slug: slug.trim() })
      rememberAndSelect({ id: tenant.id, name: tenant.name, slug: tenant.slug })
      onCreated(tenant)
    } catch (err) {
      setError(err instanceof ApiError ? err.detail || err.title : 'Unable to create the tenant. Please try again.')
    }
  }

  return (
    <form onSubmit={handleSubmit(onSubmit)} noValidate>
      {error && (
        <div className="mb-4 rounded-md bg-danger/10 px-4 py-3 text-sm text-danger" role="alert">
          {error}
        </div>
      )}

      <div className="mb-5">
        <label htmlFor="tenant-name" className="form-label">
          Tenant name&nbsp;<span className="text-danger">*</span>
        </label>
        <input
          id="tenant-name"
          type="text"
          placeholder="Acme Inc."
          className={cn('form-input', errors.name && '!border-danger')}
          {...register('name', {
            onChange: (e) => {
              // Auto-fill the slug from the name until the user edits it.
              if (!slugEdited) setValue('slug', slugify(e.target.value), { shouldValidate: false })
            },
          })}
        />
        {errors.name && <p className="text-danger mt-1 text-sm">{errors.name.message}</p>}
      </div>

      <div className="mb-6">
        <label htmlFor="tenant-slug" className="form-label">
          Slug&nbsp;<span className="text-danger">*</span>
        </label>
        <input
          id="tenant-slug"
          type="text"
          placeholder="acme"
          className={cn('form-input', errors.slug && '!border-danger')}
          {...register('slug', { onChange: () => setSlugEdited(true) })}
        />
        {errors.slug ? (
          <p className="text-danger mt-1 text-sm">{errors.slug.message}</p>
        ) : (
          <p className="text-default-400 mt-1 text-xs">A short URL-friendly identifier. Fixed once created.</p>
        )}
      </div>

      <button type="submit" disabled={isSubmitting} className="btn bg-primary font-semibold text-white hover:bg-primary-hover">
        {isSubmitting ? 'Creating…' : submitLabel}
      </button>
    </form>
  )
}

export default CreateTenantForm
