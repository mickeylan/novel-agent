import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    name: 'projects',
    component: () => import('../views/Projects.vue')
  },
  {
    path: '/projects/new',
    name: 'project-new',
    component: () => import('../views/ProjectNew.vue')
  },
  {
    path: '/projects/:id',
    name: 'project-detail',
    component: () => import('../views/ProjectDetail.vue'),
    children: [
      {
        path: '',
        redirect: 'overview'
      },
      {
        path: 'overview',
        name: 'overview',
        component: () => import('../views/Overview.vue')
      },
      {
        path: 'chapters',
        name: 'chapters',
        component: () => import('../views/Chapters.vue')
      },
      {
        path: 'characters',
        name: 'characters',
        component: () => import('../views/Characters.vue')
      },
      {
        path: 'outline',
        name: 'outline',
        component: () => import('../views/Outline.vue')
      },
      {
        path: 'writing',
        name: 'writing',
        component: () => import('../views/WritingStudio.vue')
      }
    ]
  },
  {
    path: '/search',
    name: 'search',
    component: () => import('../views/Search.vue')
  }
]

export default createRouter({
  history: createWebHistory(),
  routes
})
