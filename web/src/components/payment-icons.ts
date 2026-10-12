/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { IconType } from 'react-icons'
import {
  SiAfterpay,
  SiAlipay,
  SiApplepay,
  SiBinance,
  SiBitcoin,
  SiBitcoincash,
  SiCashapp,
  SiCoinbase,
  SiDiscord,
  SiDogecoin,
  SiEthereum,
  SiFacebook,
  SiGithub,
  SiGitlab,
  SiGooglepay,
  SiKucoin,
  SiLitecoin,
  SiLinux,
  SiMastercard,
  SiPatreon,
  SiPaypal,
  SiPaytm,
  SiRazorpay,
  SiReddit,
  SiSamsungpay,
  SiSolana,
  SiStripe,
  SiTelegram,
  SiTether,
  SiVisa,
  SiWechat,
  SiWesternunion,
  SiWise,
  SiXrp,
} from 'react-icons/si'
import {
  LuBadgeDollarSign,
  LuBadgeEuro,
  LuBanknote,
  LuCircleDollarSign,
  LuCoins,
  LuCreditCard,
  LuDollarSign,
  LuEuro,
  LuHandCoins,
  LuLandmark,
  LuPiggyBank,
  LuReceipt,
  LuWallet,
  LuWalletCards,
  LuWalletMinimal,
} from 'react-icons/lu'

/**
 * Icon table for the payment-method icon picker.
 *
 * This module is loaded on demand from `react-icon-by-name.tsx`. It must stay
 * out of the first-paint bundle, so only that component may import it, and only
 * via a dynamic `import()`.
 *
 * Why a fixed table instead of resolving any `react-icons` name at runtime:
 * a dynamic `import('react-icons/<pack>')` cannot be tree-shaken, so every pack
 * referenced that way ships in full. Measured via chunk attribution on
 * 2026-10-12, the previous per-pack dynamic loader cost ~24 MB raw in
 * `dist/static/js` (gi 6.5 MB, pi 5.1 MB, si 4.9 MB, tb 3.0 MB, md 1.9 MB,
 * fa 2.9 MB), while a named static import costs roughly 1.5 KB per icon.
 *
 * Why these two packs only (`si` and `lu`): they are the only packs whose names
 * appear anywhere in this repository. Inventory taken 2026-10-12 across the
 * frontend and backend default config:
 *   - `si` — `SiAlipay` / `SiWechat` / `SiStripe` (payment defaults in
 *     `payment-method-dialog.tsx`, `payment-methods-visual-editor.tsx`, and
 *     `setting/operation_setting/payment_setting_old.go`) plus `SiGithub` /
 *     `SiLinux` / `SiDiscord` (static imports in the security, users, and OAuth
 *     screens).
 *   - `lu` — `LuCreditCard` (the `waffo_pancake` default), which arrives as a
 *     *string* config value, not an import, so it is easy to miss.
 * The remaining 29 packs (`ai` bi bs cg ci di fa fa6 fc fi gi go gr hi hi2 im
 * io io5 lia md pi ri rx sl tb tfi ti vsc wi`) had zero references and were
 * removed; re-adding any of them costs multiple MB, so do it deliberately.
 *
 * Adding a new icon: import it by name at the top of this file, add it to
 * `ICON_BY_NAME` below, and use that exact `react-icons` component name in the
 * payment-method "Icon" field. Names outside this table render nothing. The
 * field's help text in the locales mirrors this contract, so update it too when
 * the supported set changes.
 */
export const ICON_BY_NAME: Record<string, IconType> = {
  // Payment providers
  SiAfterpay,
  SiAlipay,
  SiApplepay,
  SiCashapp,
  SiGooglepay,
  SiKucoin,
  SiMastercard,
  SiPaypal,
  SiPaytm,
  SiRazorpay,
  SiSamsungpay,
  SiStripe,
  SiVisa,
  SiWechat,
  SiWesternunion,
  SiWise,
  LuBadgeDollarSign,
  LuBadgeEuro,
  LuBanknote,
  LuCircleDollarSign,
  LuCoins,
  LuCreditCard,
  LuDollarSign,
  LuEuro,
  LuHandCoins,
  LuLandmark,
  LuPiggyBank,
  LuReceipt,
  LuWallet,
  LuWalletCards,
  LuWalletMinimal,
  // Crypto
  SiBinance,
  SiBitcoin,
  SiBitcoincash,
  SiCoinbase,
  SiDogecoin,
  SiEthereum,
  SiLitecoin,
  SiSolana,
  SiTether,
  SiXrp,
  // Platforms / communities commonly used by deployments
  SiDiscord,
  SiFacebook,
  SiGithub,
  SiGitlab,
  SiLinux,
  SiPatreon,
  SiReddit,
  SiTelegram,
}
