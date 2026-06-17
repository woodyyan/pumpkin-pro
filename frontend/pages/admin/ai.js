import AdminShell from '../../components/admin/AdminShell'
import { AdminAIPage } from '../../components/admin/AdminSections'

export default function AdminAIRoute() {
  return (
    <AdminShell section="ai">
      {({ onUnauthorized }) => <AdminAIPage onUnauthorized={onUnauthorized} />}
    </AdminShell>
  )
}
