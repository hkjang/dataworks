import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { blockedGateFixture, productFixture } from '@/test/dataworks-fixtures'
import { LifecycleStepper } from './lifecycle-stepper'

describe('LifecycleStepper', () => {
  it('derives the current blocked stage from workspace evidence', () => {
    render(
      <LifecycleStepper
        data={{
          product: productFixture,
          canvas: {
            product_key: productFixture.product_key,
            customer_problem: 'problem',
            buyer: 'buyer',
            use_cases: 'use cases',
            provided_data: 'FIN_TX',
            differentiation: '',
            pricing_model: 'usage',
            risk_notes: '',
            poc_success_criteria: '',
            expected_revenue: '',
            owner: 'data-team',
            updated_by: 'admin',
            created_at: '',
            updated_at: '',
          },
          canvasDraft: false,
          gate: blockedGateFixture,
          evidence: {
            product_key: productFixture.product_key,
            pack_json: '{}',
            artifact_ref: '',
            created_by: 'admin',
            created_at: '',
            updated_at: '',
          },
        }}
      />,
    )

    expect(screen.getByLabelText('승인: 차단됨')).toBeInTheDocument()
    expect(screen.getByLabelText('자산: 완료')).toBeInTheDocument()
    expect(screen.getByText('운영')).toBeInTheDocument()
  })
})
