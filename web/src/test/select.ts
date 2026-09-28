import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

export async function selectOption(control: HTMLElement, value: string) {
  await userEvent.click(control)
  const option = screen.getAllByRole('option').find(item => item.dataset.value === value)
  if (!option) throw new Error(`Missing option: ${value}`)
  await userEvent.click(option)
}
