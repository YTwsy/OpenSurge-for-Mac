import { t } from '../i18n'
import type { Login } from '../hooks/useDesktopUtilities'

export function LoginItemFeedback({ login, hintClass, errorClass, openSettings }: {
 login: Login | undefined
 hintClass: string
 errorClass: string
 openSettings: () => void
}) {
 if (!login) return null
 return <>
  {login.state === 'approval' && <p className={hintClass}>{t('等待 macOS 批准登录项。')}</p>}
  {login.state === 'not_found' && <p className={hintClass}>{t('macOS 未找到当前版本的登录项。开启“登录时显示”可重新注册。')}</p>}
  {login.state === 'unavailable' && <p className={hintClass}>{t('当前 App 无法管理登录项。请从已安装的应用中设置。')}</p>}
  {login.failed && <p role="alert" className={errorClass}>{t('登录项未能更新，已保留 macOS 的实际状态。')}{login.error && <> {login.error}</>}</p>}
  {(login.state === 'approval' || login.state === 'not_found' || login.failed) && <p className={hintClass}><button onClick={openSettings}>{t('打开登录项设置')}</button></p>}
 </>
}
