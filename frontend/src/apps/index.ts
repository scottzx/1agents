// Installable-app bundle aggregator (Epic #317). Importing this module runs each
// app's index side-effects — registerAppView(...) for every mount-point view its
// manifest (served by GET /api/apps) declares. The platform's MountPointRenderer
// resolves those view names to the registered components.
//
// Wave 3 apps: 自媒体 (project-tab) · CRM (L1 page + lens) · AI 电台 (L1 page, opt-in).
import './media';
import './crm';
import './radio';
