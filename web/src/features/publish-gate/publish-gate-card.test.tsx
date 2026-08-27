import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { blockedGateFixture } from '@/test/dataworks-fixtures'
import { PublishGateCard } from './publish-gate-card'

describe('출시 게이트', () => {
  it('서버 게이트 판정과 차단 사유를 표시한다', () => {
    render(<PublishGateCard gate={blockedGateFixture} />)

    expect(screen.getByText('출시 게이트 · G-01')).toBeInTheDocument()
    expect(screen.getByText(/구성 완료도/)).toBeInTheDocument()
    expect(screen.getByText('출시 차단됨')).toBeInTheDocument()
    expect(screen.getByText('필수 승인 누락: 법무 승인')).toBeInTheDocument()
    expect(screen.getByText('법무 승인')).toBeInTheDocument()
    expect(screen.getAllByText('대기')).toHaveLength(1)
  })
})
