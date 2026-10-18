import { createRouter, createWebHashHistory } from 'vue-router'

/**
 * 打包后页面以 file:// 加载，只能用 hash 路由；
 * 开发模式下同样是 hash，行为一致，不会出现刷新 404。
 */
const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/dashboard' },
    {
      path: '/dashboard',
      name: 'dashboard',
      component: () => import('./views/DashboardView.vue'),
      meta: { title: '运行监控' },
    },
    {
      path: '/new',
      name: 'new',
      component: () => import('./views/NewTestView.vue'),
      meta: { title: '新建压测' },
    },
    {
      path: '/jobs/:id',
      name: 'job-detail',
      component: () => import('./views/JobDetailView.vue'),
      meta: { title: '任务详情' },
    },
    {
      path: '/history',
      name: 'history',
      component: () => import('./views/HistoryView.vue'),
      meta: { title: '历史记录' },
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('./views/SettingsView.vue'),
      meta: { title: '设置' },
    },
    { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
  ],
})

export default router
