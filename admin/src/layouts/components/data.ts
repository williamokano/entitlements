import { type MenuItemType } from '@/types'

export const menuItems: MenuItemType[] = [
  {
    'icon': 'dashboard',
    'slug': 'main',
    'label': 'Main',
    'isTitle': true,
    'children': [
      {
        'icon': 'dashboard',
        'slug': 'dashboards',
        'label': 'Dashboards',
        'children': [
          {
            'url': '/demo/dashboard/ecommerce',
            'slug': 'pages:dashboard-ecommerce',
            'label': 'Ecommerce',
          },
          {
            'url': '/demo/dashboard/analytics',
            'slug': 'pages:dashboard-analytics',
            'label': 'Analytics',
          },
          {
            'url': '/demo/dashboard/projects',
            'slug': 'pages:dashboard-projects',
            'label': 'Projects',
          },
        ],
      },
      {
        'url': '/demo/landing',
        'icon': 'rocket',
        'slug': 'pages:landing',
        'label': 'Landing',
      },
    ],
  },
  {
    'icon': 'apps',
    'slug': 'apps',
    'label': 'Apps',
    'isTitle': true,
    'children': [
      {
        'icon': 'basket',
        'slug': 'ecommerce',
        'label': 'Ecommerce',
        'children': [
          {
            'url': '/demo/apps/ecommerce/marketplace',
            'icon': 'building-store',
            'slug': 'pages:apps-ecommerce-marketplace',
            'label': 'Marketplace',
          },
          {
            'icon': 'shirt-sport',
            'slug': 'products',
            'label': 'Products',
            'children': [
              {
                'url': '/demo/apps/ecommerce/products',
                'slug': 'pages:apps-ecommerce-products',
                'label': 'Products',
              },
              {
                'url': '/demo/apps/ecommerce/products-grid',
                'slug': 'pages:apps-ecommerce-products-grid',
                'label': 'Products Grid',
              },
              {
                'url': '/demo/apps/ecommerce/product-details',
                'slug': 'pages:apps-ecommerce-product-details',
                'label': 'Product Details',
              },
              {
                'url': '/demo/apps/ecommerce/product-add',
                'slug': 'pages:apps-ecommerce-product-add',
                'label': 'Add Product',
              },
            ],
          },
          {
            'url': '/demo/apps/ecommerce/categories',
            'icon': 'category',
            'slug': 'pages:apps-ecommerce-categories',
            'label': 'Categories',
          },
          {
            'icon': 'shopping-bag',
            'slug': 'orders',
            'label': 'Orders',
            'children': [
              {
                'url': '/demo/apps/ecommerce/orders',
                'slug': 'pages:apps-ecommerce-orders',
                'label': 'Orders',
              },
              {
                'url': '/demo/apps/ecommerce/order-details',
                'slug': 'pages:apps-ecommerce-order-details',
                'label': 'Order Details',
              },
              {
                'url': '/demo/apps/ecommerce/order-add',
                'slug': 'pages:apps-ecommerce-order-add',
                'label': 'Add/Edit Order',
              },
            ],
          },
          {
            'url': '/demo/apps/ecommerce/customers',
            'icon': 'users',
            'slug': 'pages:apps-ecommerce-customers',
            'label': 'Customers',
          },
          {
            'url': '/demo/apps/ecommerce/cart',
            'icon': 'shopping-cart',
            'slug': 'pages:apps-ecommerce-cart',
            'label': 'Cart',
          },
          {
            'url': '/demo/apps/ecommerce/checkout',
            'icon': 'shopping-bag-heart',
            'slug': 'pages:apps-ecommerce-checkout',
            'label': 'Checkout',
          },
          {
            'icon': 'users-group',
            'slug': 'sellers',
            'label': 'Sellers',
            'children': [
              {
                'url': '/demo/apps/ecommerce/sellers',
                'slug': 'pages:apps-ecommerce-sellers',
                'label': 'Sellers',
              },
              {
                'url': '/demo/apps/ecommerce/seller-details',
                'slug': 'pages:apps-ecommerce-seller-details',
                'label': 'Sellers Details',
              },
            ],
          },
          {
            'url': '/demo/apps/ecommerce/refunds',
            'icon': 'credit-card-refund',
            'slug': 'pages:apps-ecommerce-refunds',
            'label': 'Refunds',
          },
          {
            'url': '/demo/apps/ecommerce/reviews',
            'icon': 'message-star',
            'slug': 'pages:apps-ecommerce-reviews',
            'label': 'Reviews',
          },
          {
            'icon': 'building-warehouse',
            'slug': 'inventory',
            'label': 'Inventory',
            'children': [
              {
                'url': '/demo/apps/ecommerce/warehouse',
                'slug': 'pages:apps-ecommerce-warehouse',
                'label': 'Warehouse',
              },
              {
                'url': '/demo/apps/ecommerce/product-stocks',
                'slug': 'pages:apps-ecommerce-product-stocks',
                'label': 'Product Stocks',
              },
              {
                'url': '/demo/apps/ecommerce/purchased-orders',
                'slug': 'pages:apps-ecommerce-purchased-orders',
                'label': 'Purchased Orders',
              },
            ],
          },
          {
            'icon': 'report',
            'slug': 'reports',
            'label': 'Reports',
            'children': [
              {
                'url': '/demo/apps/ecommerce/product-views',
                'slug': 'pages:apps-ecommerce-product-views',
                'label': 'Product Views',
              },
              {
                'url': '/demo/apps/ecommerce/sales',
                'slug': 'pages:apps-ecommerce-sales',
                'label': 'Sales',
              },
            ],
          },
          {
            'url': '/demo/apps/ecommerce/attributes',
            'icon': 'wand',
            'slug': 'pages:apps-ecommerce-attributes',
            'label': 'Attributes',
          },
          {
            'url': '/demo/apps/ecommerce/settings',
            'icon': 'settings',
            'slug': 'pages:apps-ecommerce-settings',
            'label': 'Settings',
          },
        ],
      },
      {
        'icon': 'mailbox',
        'slug': 'email',
        'badge': {
          'text': 'New',
          'className': 'bg-danger text-white',
        },
        'label': 'Email',
        'children': [
          {
            'url': '/demo/apps/email/inbox',
            'slug': 'pages:apps-email-inbox',
            'label': 'Inbox',
          },
          {
            'url': '/demo/apps/email/details',
            'slug': 'pages:apps-email-details',
            'label': 'Details',
          },
          {
            'url': '/demo/apps/email/compose',
            'slug': 'pages:apps-email-compose',
            'label': 'Compose',
          },
        ],
      },
      {
        'icon': 'users',
        'slug': 'users',
        'label': 'Users',
        'children': [
          {
            'url': '/demo/apps/users/contacts',
            'slug': 'pages:apps-users-contacts',
            'label': 'Contacts',
          },
          {
            'url': '/demo/apps/users/roles',
            'slug': 'pages:apps-users-roles',
            'label': 'Roles',
          },
          {
            'url': '/demo/apps/users/role-details',
            'slug': 'pages:apps-users-role-details',
            'label': 'Role Details',
          },
          {
            'url': '/demo/apps/users/permissions',
            'slug': 'pages:apps-users-permissions',
            'label': 'Permissions',
          },
        ],
      },
      {
        'icon': 'briefcase',
        'slug': 'projects',
        'label': 'Projects',
        'children': [
          {
            'url': '/demo/apps/projects/grid',
            'slug': 'pages:apps-projects-grid',
            'label': 'My Projects',
          },
          {
            'url': '/demo/apps/projects/list',
            'slug': 'pages:apps-projects-list',
            'label': 'Projects List',
          },
          {
            'url': '/demo/apps/projects/details',
            'slug': 'pages:apps-projects-details',
            'label': 'View Project',
          },
          {
            'url': '/demo/apps/projects/kanban',
            'slug': 'pages:apps-projects-kanban',
            'label': 'Kanban Board',
          },
          {
            'url': '/demo/apps/projects/team-board',
            'slug': 'pages:apps-projects-team-board',
            'label': 'Team Board',
          },
          {
            'url': '/demo/apps/projects/activity',
            'slug': 'pages:apps-projects-activity',
            'label': 'Activity Steam',
          },
        ],
      },
      {
        'url': '/demo/apps/file-manager',
        'icon': 'folder-open',
        'slug': 'pages:apps-file-manager',
        'label': 'File Manager',
      },
      {
        'url': '/demo/apps/chat',
        'icon': 'message',
        'slug': 'pages:apps-chat',
        'label': 'Chat',
      },
      {
        'url': '/demo/apps/calendar',
        'icon': 'calendar',
        'slug': 'pages:apps-calendar',
        'label': 'Calendar',
      },
      {
        'url': '/demo/apps/social-feed',
        'icon': 'rss',
        'slug': 'pages:apps-social-feed',
        'label': 'Social Feed',
      },
      {
        'icon': 'invoice',
        'slug': 'invoice',
        'label': 'Invoice',
        'children': [
          {
            'url': '/demo/apps/invoice/list',
            'slug': 'pages:apps-invoice-list',
            'label': 'Invoices',
          },
          {
            'url': '/demo/apps/invoice/details',
            'slug': 'pages:apps-invoice-details',
            'label': 'Single Invoice',
          },
          {
            'url': '/demo/apps/invoice/create',
            'slug': 'pages:apps-invoice-create',
            'label': 'New Invoice',
          },
        ],
      },
      {
        'url': '/demo/apps/companies',
        'icon': 'building',
        'slug': 'pages:apps-companies',
        'label': 'Companies',
      },
      {
        'icon': 'apps',
        'slug': 'more-apps',
        'label': 'More Apps',
        'children': [
          {
            'url': '/demo/apps/clients',
            'icon': 'users-group',
            'slug': 'pages:apps-clients',
            'label': 'Clients',
          },
          {
            'url': '/demo/apps/outlook',
            'icon': 'layout-cards',
            'slug': 'pages:apps-outlook',
            'label': 'Outlook View',
          },
          {
            'url': '/demo/apps/vote-list',
            'icon': 'caret-up-down',
            'slug': 'pages:apps-vote-list',
            'label': 'Vote List',
          },
          {
            'url': '/demo/apps/issue-tracker',
            'icon': 'bug',
            'slug': 'pages:apps-issue-tracker',
            'label': 'Issue Tracker',
          },
          {
            'url': '/demo/apps/api-keys',
            'icon': 'key',
            'slug': 'pages:apps-api-keys',
            'label': 'API Keys',
          },
          {
            'url': '/demo/apps/manage',
            'icon': 'apps',
            'slug': 'pages:apps-manage',
            'label': 'Manage Apps',
          },
          {
            'icon': 'article',
            'slug': 'blog',
            'label': 'Blog',
            'children': [
              {
                'url': '/demo/apps/blog/list',
                'slug': 'pages:apps-blog-list',
                'label': 'Blog List',
              },
              {
                'url': '/demo/apps/blog/grid',
                'slug': 'pages:apps-blog-grid',
                'label': 'Blog Grid',
              },
              {
                'url': '/demo/apps/blog/article',
                'slug': 'pages:apps-blog-article',
                'label': 'Article',
              },
              {
                'url': '/demo/apps/blog/add',
                'slug': 'pages:apps-blog-add',
                'label': 'Add Article',
              },
            ],
          },
          {
            'url': '/demo/apps/pin-board',
            'icon': 'pin',
            'slug': 'pages:apps-pin-board',
            'label': 'Pin Board',
          },
          {
            'icon': 'messages',
            'slug': 'forum',
            'label': 'Forum',
            'children': [
              {
                'url': '/demo/apps/forum/view',
                'slug': 'pages:apps-forum-view',
                'label': 'Forum View',
              },
              {
                'url': '/demo/apps/forum/post',
                'slug': 'pages:apps-forum-post',
                'label': 'Forum Post',
              },
            ],
          },
        ],
      },
    ],
  },
  {
    'icon': 'files',
    'slug': 'custom-pages',
    'label': 'Custom Pages',
    'isTitle': true,
    'children': [
      {
        'icon': 'files',
        'slug': 'pages',
        'label': 'Pages',
        'children': [
          {
            'url': '/demo/pages/profile',
            'slug': 'pages:pages-profile',
            'label': 'Profile',
          },
          {
            'url': '/demo/pages/account-settings',
            'slug': 'pages:pages-account-settings',
            'label': 'Account Settings',
          },
          {
            'url': '/demo/pages/faq',
            'slug': 'pages:pages-faq',
            'label': 'FAQ',
          },
          {
            'url': '/demo/pages/pricing',
            'slug': 'pages:pages-pricing',
            'label': 'Pricing',
          },
          {
            'url': '/demo/pages/empty',
            'slug': 'pages:pages-empty',
            'label': 'Empty Page',
          },
          {
            'url': '/demo/pages/timeline',
            'slug': 'pages:pages-timeline',
            'label': 'Timeline',
          },
          {
            'url': '/demo/pages/gallery',
            'slug': 'pages:pages-gallery',
            'label': 'Gallery',
          },
          {
            'url': '/demo/pages/sitemap',
            'slug': 'pages:pages-sitemap',
            'label': 'Sitemap',
          },
          {
            'url': '/demo/pages/search-results',
            'slug': 'pages:pages-search-results',
            'label': 'Search Results',
          },
          {
            'url': '/demo/pages/coming-soon',
            'slug': 'pages:pages-coming-soon',
            'label': 'Coming Soon',
          },
          {
            'url': '/demo/pages/privacy-policy',
            'slug': 'pages:pages-privacy-policy',
            'label': 'Privacy Policy',
          },
          {
            'url': '/demo/pages/terms-conditions',
            'slug': 'pages:pages-terms-conditions',
            'label': 'Terms & Conditions',
          },
        ],
      },
      {
        'icon': 'cpu',
        'slug': 'plugins',
        'label': 'Plugins',
        'children': [
          {
            'url': '/demo/plugins/sortable',
            'slug': 'pages:plugins-sortable',
            'label': 'Sortable List',
          },
          {
            'url': '/demo/plugins/text-diff',
            'slug': 'pages:plugins-text-diff',
            'label': 'Text Diff',
          },
          {
            'url': '/demo/plugins/pdf-viewer',
            'slug': 'pages:plugins-pdf-viewer',
            'label': 'PDF Viewer',
          },
          {
            'url': '/demo/plugins/sweet-alerts',
            'slug': 'pages:plugins-sweet-alerts',
            'label': 'Sweet Alerts',
          },
          {
            'url': '/demo/plugins/idle-timer',
            'slug': 'pages:plugins-idle-timer',
            'label': 'Idle Timer',
          },
          {
            'url': '/demo/plugins/pass-meter',
            'slug': 'pages:plugins-pass-meter',
            'label': 'Password Meter',
          },
          {
            'url': '/demo/plugins/clipboard',
            'slug': 'pages:plugins-clipboard',
            'label': 'Clipboard',
          },
          {
            'url': '/demo/plugins/tree-view',
            'slug': 'pages:plugins-tree-view',
            'label': 'Tree View',
          },
          {
            'url': '/demo/plugins/loading-buttons',
            'slug': 'pages:plugins-loading-buttons',
            'label': 'Loading Buttons',
          },
          {
            'url': '/demo/plugins/masonry',
            'slug': 'pages:plugins-masonry',
            'label': 'Masonry',
          },
          {
            'url': '/demo/plugins/tour',
            'slug': 'pages:plugins-tour',
            'label': 'Tour',
          },
          {
            'url': '/demo/plugins/animation',
            'slug': 'pages:plugins-animation',
            'label': 'Animation',
          },
          {
            'url': '/demo/plugins/video-player',
            'slug': 'pages:plugins-video-player',
            'label': 'Video Player',
          },
        ],
      },
      {
        'icon': 'password-user',
        'slug': 'authentication',
        'label': 'Authentication',
        'children': [
          {
            'slug': 'auth-basic',
            'label': 'Basic',
            'children': [
              {
                'url': '/demo/auth/sign-in',
                'slug': 'pages:auth-sign-in',
                'label': 'Sign In',
              },
              {
                'url': '/demo/auth/sign-up',
                'slug': 'pages:auth-sign-up',
                'label': 'Sign Up',
              },
              {
                'url': '/demo/auth/reset-pass',
                'slug': 'pages:auth-reset-pass',
                'label': 'Reset Password',
              },
              {
                'url': '/demo/auth/new-pass',
                'slug': 'pages:auth-new-pass',
                'label': 'New Password',
              },
              {
                'url': '/demo/auth/two-factor',
                'slug': 'pages:auth-two-factor',
                'label': 'Two Factor',
              },
              {
                'url': '/demo/auth/lock-screen',
                'slug': 'pages:auth-lock-screen',
                'label': 'Lock Screen',
              },
              {
                'url': '/demo/auth/success-mail',
                'slug': 'pages:auth-success-mail',
                'label': 'Success Mail',
              },
              {
                'url': '/demo/auth/login-pin',
                'slug': 'pages:auth-login-pin',
                'label': 'Login with PIN',
              },
              {
                'url': '/demo/auth/delete-account',
                'slug': 'pages:auth-delete-account',
                'label': 'Delete Account',
              },
            ],
          },
          {
            'slug': 'auth-card',
            'label': 'Card',
            'children': [
              {
                'url': '/demo/auth/card/sign-in',
                'slug': 'pages:auth-card-sign-in',
                'label': 'Sign In',
              },
              {
                'url': '/demo/auth/card/sign-up',
                'slug': 'pages:auth-card-sign-up',
                'label': 'Sign Up',
              },
              {
                'url': '/demo/auth/card/reset-pass',
                'slug': 'pages:auth-card-reset-pass',
                'label': 'Reset Password',
              },
              {
                'url': '/demo/auth/card/new-pass',
                'slug': 'pages:auth-card-new-pass',
                'label': 'New Password',
              },
              {
                'url': '/demo/auth/card/two-factor',
                'slug': 'pages:auth-card-two-factor',
                'label': 'Two Factor',
              },
              {
                'url': '/demo/auth/card/lock-screen',
                'slug': 'pages:auth-card-lock-screen',
                'label': 'Lock Screen',
              },
              {
                'url': '/demo/auth/card/success-mail',
                'slug': 'pages:auth-card-success-mail',
                'label': 'Success Mail',
              },
              {
                'url': '/demo/auth/card/login-pin',
                'slug': 'pages:auth-card-login-pin',
                'label': 'Login with PIN',
              },
              {
                'url': '/demo/auth/card/delete-account',
                'slug': 'pages:auth-card-delete-account',
                'label': 'Delete Account',
              },
            ],
          },
          {
            'slug': 'auth-split',
            'label': 'Split',
            'children': [
              {
                'url': '/demo/auth/split/sign-in',
                'slug': 'pages:auth-split-sign-in',
                'label': 'Sign In',
              },
              {
                'url': '/demo/auth/split/sign-up',
                'slug': 'pages:auth-split-sign-up',
                'label': 'Sign Up',
              },
              {
                'url': '/demo/auth/split/reset-pass',
                'slug': 'pages:auth-split-reset-pass',
                'label': 'Reset Password',
              },
              {
                'url': '/demo/auth/split/new-pass',
                'slug': 'pages:auth-split-new-pass',
                'label': 'New Password',
              },
              {
                'url': '/demo/auth/split/two-factor',
                'slug': 'pages:auth-split-two-factor',
                'label': 'Two Factor',
              },
              {
                'url': '/demo/auth/split/lock-screen',
                'slug': 'pages:auth-split-lock-screen',
                'label': 'Lock Screen',
              },
              {
                'url': '/demo/auth/split/success-mail',
                'slug': 'pages:auth-split-success-mail',
                'label': 'Success Mail',
              },
              {
                'url': '/demo/auth/split/login-pin',
                'slug': 'pages:auth-split-login-pin',
                'label': 'Login with PIN',
              },
              {
                'url': '/demo/auth/split/delete-account',
                'slug': 'pages:auth-split-delete-account',
                'label': 'Delete Account',
              },
            ],
          },
        ],
      },
      {
        'icon': 'alert-triangle',
        'slug': 'error-pages',
        'label': 'Error Pages',
        'children': [
          {
            'url': '/demo/error/400',
            'slug': 'pages:error-400',
            'label': '400 Bad Request',
          },
          {
            'url': '/demo/error/401',
            'slug': 'pages:error-401',
            'label': '401 Unauthorized',
          },
          {
            'url': '/demo/error/403',
            'slug': 'pages:error-403',
            'label': '403 Forbidden',
          },
          {
            'url': '/demo/error/404',
            'slug': 'pages:error-404',
            'label': '404 Not Found',
          },
          {
            'url': '/demo/error/408',
            'slug': 'pages:error-408',
            'label': '408 Request Timeout',
          },
          {
            'url': '/demo/error/500',
            'slug': 'pages:error-500',
            'label': '500 Internal Server',
          },
          {
            'url': '/demo/error/maintenance',
            'slug': 'pages:error-maintenance',
            'label': 'Maintenance',
          },
        ],
      },
    ],
  },
  {
    'icon': 'table-column',
    'slug': 'layouts',
    'label': 'Layouts',
    'isTitle': true,
    'children': [
      {
        'icon': 'layout',
        'slug': 'layout-options',
        'label': 'Layout Options',
        'children': [
          {
            'url': '/demo/layouts/scrollable',
            'slug': 'pages:layouts-scrollable',
            'label': 'Scrollable',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/compact',
            'slug': 'pages:layouts-compact',
            'label': 'Compact',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/boxed',
            'slug': 'pages:layouts-boxed',
            'label': 'Boxed',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/horizontal',
            'slug': 'pages:layouts-horizontal',
            'label': 'Horizontal',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/preloader',
            'slug': 'pages:layouts-preloader',
            'label': 'Preloader',
            'target': '_blank',
          },
        ],
      },
      {
        'icon': 'layout-sidebar-inactive',
        'slug': 'sidebars',
        'label': 'Sidebars',
        'children': [
          {
            'url': '/demo/layouts/sidebar-light',
            'slug': 'pages:layouts-sidebar-light',
            'label': 'Light Menu',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/sidebar-gradient',
            'slug': 'pages:layouts-sidebar-gradient',
            'label': 'Gradient Menu',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/sidebar-gray',
            'slug': 'pages:layouts-sidebar-gray',
            'label': 'Gray Menu',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/sidebar-image',
            'slug': 'pages:layouts-sidebar-image',
            'label': 'Image Menu',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/sidebar-compact',
            'slug': 'pages:layouts-sidebar-compact',
            'label': 'Compact Menu',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/sidebar-on-hover',
            'slug': 'pages:layouts-sidebar-on-hover',
            'label': 'On Hover Menu',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/sidebar-on-hover-active',
            'slug': 'pages:layouts-sidebar-on-hover-active',
            'label': 'On Hover Active',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/sidebar-offcanvas',
            'slug': 'pages:layouts-sidebar-offcanvas',
            'label': 'Offcanvas Menu',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/sidebar-no-icons',
            'slug': 'pages:layouts-sidebar-no-icons',
            'label': 'No Icons with Lines',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/sidebar-with-lines',
            'slug': 'pages:layouts-sidebar-with-lines',
            'label': 'Sidebar with Lines',
            'target': '_blank',
          },
        ],
      },
      {
        'icon': 'layout-bottombar',
        'slug': 'topbar',
        'label': 'Topbar',
        'children': [
          {
            'url': '/demo/layouts/topbar-dark',
            'slug': 'pages:layouts-topbar-dark',
            'label': 'Dark Topbar',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/topbar-gray',
            'slug': 'pages:layouts-topbar-gray',
            'label': 'Gray Topbar',
            'target': '_blank',
          },
          {
            'url': '/demo/layouts/topbar-gradient',
            'slug': 'pages:layouts-topbar-gradient',
            'label': 'Gradient Topbar',
            'target': '_blank',
          },
        ],
      },
    ],
  },
  {
    'icon': 'components',
    'slug': 'components',
    'label': 'Components',
    'isTitle': true,
    'children': [
      {
        'icon': 'components',
        'slug': 'base-ui',
        'label': 'Base UI',
        'children': [
          {
            'url': '/demo/ui/accordions',
            'slug': 'pages:ui-accordions',
            'label': 'Accordions',
          },
          {
            'url': '/demo/ui/alerts',
            'slug': 'pages:ui-alerts',
            'label': 'Alerts',
          },
          {
            'url': '/demo/ui/images',
            'slug': 'pages:ui-images',
            'label': 'Images',
          },
          {
            'url': '/demo/ui/badges',
            'slug': 'pages:ui-badges',
            'label': 'Badges',
          },
          {
            'url': '/demo/ui/breadcrumb',
            'slug': 'pages:ui-breadcrumb',
            'label': 'Breadcrumb',
          },
          {
            'url': '/demo/ui/buttons',
            'slug': 'pages:ui-buttons',
            'label': 'Buttons',
          },
          {
            'url': '/demo/ui/cards',
            'slug': 'pages:ui-cards',
            'label': 'Cards',
          },
          {
            'url': '/demo/ui/carousel',
            'slug': 'pages:ui-carousel',
            'label': 'Carousel',
          },
          {
            'url': '/demo/ui/collapse',
            'slug': 'pages:ui-collapse',
            'label': 'Collapse',
          },
          {
            'url': '/demo/ui/colors',
            'slug': 'pages:ui-colors',
            'label': 'Colors',
          },
          {
            'url': '/demo/ui/dropdowns',
            'slug': 'pages:ui-dropdowns',
            'label': 'Dropdowns',
          },
          {
            'url': '/demo/ui/videos',
            'slug': 'pages:ui-videos',
            'label': 'Videos',
          },
          {
            'url': '/demo/ui/links',
            'slug': 'pages:ui-links',
            'label': 'Links',
          },
          {
            'url': '/demo/ui/list-group',
            'slug': 'pages:ui-list-group',
            'label': 'List Group',
          },
          {
            'url': '/demo/ui/modals',
            'slug': 'pages:ui-modals',
            'label': 'Modals',
          },
          {
            'url': '/demo/ui/notifications',
            'slug': 'pages:ui-notifications',
            'label': 'Notifications',
          },
          {
            'url': '/demo/ui/offcanvas',
            'slug': 'pages:ui-offcanvas',
            'label': 'Offcanvas',
          },
          {
            'url': '/demo/ui/placeholders',
            'slug': 'pages:ui-placeholders',
            'label': 'Placeholders',
          },
          {
            'url': '/demo/ui/pagination',
            'slug': 'pages:ui-pagination',
            'label': 'Pagination',
          },
          {
            'url': '/demo/ui/popovers',
            'slug': 'pages:ui-popovers',
            'label': 'Popovers',
          },
          {
            'url': '/demo/ui/progress',
            'slug': 'pages:ui-progress',
            'label': 'Progress',
          },
          {
            'url': '/demo/ui/spinners',
            'slug': 'pages:ui-spinners',
            'label': 'Spinners',
          },
          {
            'url': '/demo/ui/tabs',
            'slug': 'pages:ui-tabs',
            'label': 'Tabs',
          },
          {
            'url': '/demo/ui/tooltips',
            'slug': 'pages:ui-tooltips',
            'label': 'Tooltips',
          },
          {
            'url': '/demo/ui/typography',
            'slug': 'pages:ui-typography',
            'label': 'Typography',
          },
          {
            'url': '/demo/ui/utilities',
            'slug': 'pages:ui-utilities',
            'label': 'Utilities',
          },
        ],
      },
      {
        'url': '/demo/widgets',
        'icon': 'stack-2',
        'slug': 'pages:widgets',
        'label': 'Widgets',
      },
      {
        'url': '/demo/metrics',
        'icon': 'chart-histogram',
        'slug': 'pages:metrics',
        'label': 'Metrics',
      },
      {
        'icon': 'chart-donut',
        'slug': 'charts',
        'label': 'Charts',
        'children': [
          {
            'slug': 'apex-charts',
            'label': 'Apex Charts',
            'children': [
              {
                'url': '/demo/charts/apex/area',
                'slug': 'pages:charts-apex-area',
                'label': 'Area',
              },
              {
                'url': '/demo/charts/apex/bar',
                'slug': 'pages:charts-apex-bar',
                'label': 'Bar',
              },
              {
                'url': '/demo/charts/apex/bubble',
                'slug': 'pages:charts-apex-bubble',
                'label': 'Bubble',
              },
              {
                'url': '/demo/charts/apex/candlestick',
                'slug': 'pages:charts-apex-candlestick',
                'label': 'Candlestick',
              },
              {
                'url': '/demo/charts/apex/column',
                'slug': 'pages:charts-apex-column',
                'label': 'Column',
              },
              {
                'url': '/demo/charts/apex/heatmap',
                'slug': 'pages:charts-apex-heatmap',
                'label': 'Heatmap',
              },
              {
                'url': '/demo/charts/apex/line',
                'slug': 'pages:charts-apex-line',
                'label': 'Line',
              },
              {
                'url': '/demo/charts/apex/mixed',
                'slug': 'pages:charts-apex-mixed',
                'label': 'Mixed',
              },
              {
                'url': '/demo/charts/apex/timeline',
                'slug': 'pages:charts-apex-timeline',
                'label': 'Timeline',
              },
              {
                'url': '/demo/charts/apex/boxplot',
                'slug': 'pages:charts-apex-boxplot',
                'label': 'Boxplot',
              },
              {
                'url': '/demo/charts/apex/treemap',
                'slug': 'pages:charts-apex-treemap',
                'label': 'Treemap',
              },
              {
                'url': '/demo/charts/apex/pie',
                'slug': 'pages:charts-apex-pie',
                'label': 'Pie',
              },
              {
                'url': '/demo/charts/apex/radar',
                'slug': 'pages:charts-apex-radar',
                'label': 'Radar',
              },
              {
                'url': '/demo/charts/apex/radialbar',
                'slug': 'pages:charts-apex-radialbar',
                'label': 'RadialBar',
              },
              {
                'url': '/demo/charts/apex/scatter',
                'slug': 'pages:charts-apex-scatter',
                'label': 'Scatter',
              },
              {
                'url': '/demo/charts/apex/polar-area',
                'slug': 'pages:charts-apex-polar-area',
                'label': 'Polar Area',
              },
              {
                'url': '/demo/charts/apex/sparklines',
                'slug': 'pages:charts-apex-sparklines',
                'label': 'Sparklines',
              },
              {
                'url': '/demo/charts/apex/range',
                'slug': 'pages:charts-apex-range',
                'label': 'Range',
              },
              {
                'url': '/demo/charts/apex/funnel',
                'slug': 'pages:charts-apex-funnel',
                'label': 'Funnel',
              },
              {
                'url': '/demo/charts/apex/slope',
                'slug': 'pages:charts-apex-slope',
                'label': 'Slope',
              },
            ],
          },
          {
            'slug': 'echarts',
            'label': 'Echarts',
            'children': [
              {
                'url': '/demo/charts/echart/line',
                'slug': 'pages:charts-echart-line',
                'label': 'Line',
              },
              {
                'url': '/demo/charts/echart/bar',
                'slug': 'pages:charts-echart-bar',
                'label': 'Bar',
              },
              {
                'url': '/demo/charts/echart/pie',
                'slug': 'pages:charts-echart-pie',
                'label': 'Pie',
              },
              {
                'url': '/demo/charts/echart/scatter',
                'slug': 'pages:charts-echart-scatter',
                'label': 'Scatter',
              },
              {
                'url': '/demo/charts/echart/geo-map',
                'slug': 'pages:charts-echart-geo-map',
                'label': 'GEO Map',
              },
              {
                'url': '/demo/charts/echart/gauge',
                'slug': 'pages:charts-echart-gauge',
                'label': 'Gauge',
              },
              {
                'url': '/demo/charts/echart/candlestick',
                'slug': 'pages:charts-echart-candlestick',
                'label': 'Candlestick',
              },
              {
                'url': '/demo/charts/echart/area',
                'slug': 'pages:charts-echart-area',
                'label': 'Area',
              },
              {
                'url': '/demo/charts/echart/radar',
                'slug': 'pages:charts-echart-radar',
                'label': 'Radar',
              },
              {
                'url': '/demo/charts/echart/heatmap',
                'slug': 'pages:charts-echart-heatmap',
                'label': 'Heatmap',
              },
              {
                'url': '/demo/charts/echart/other',
                'slug': 'pages:charts-echart-other',
                'label': 'Other',
              },
            ],
          },
        ],
      },
      {
        'icon': 'clipboard-list',
        'slug': 'forms',
        'label': 'Forms',
        'children': [
          {
            'url': '/demo/form/elements',
            'slug': 'pages:form-elements',
            'label': 'Basic Elements',
          },
          {
            'url': '/demo/form/pickers',
            'slug': 'pages:form-pickers',
            'label': 'Pickers',
          },
          {
            'url': '/demo/form/select',
            'slug': 'pages:form-select',
            'label': 'Select',
          },
          {
            'url': '/demo/form/validation',
            'slug': 'pages:form-validation',
            'label': 'Validation',
          },
          {
            'url': '/demo/form/wizard',
            'slug': 'pages:form-wizard',
            'label': 'Wizard',
          },
          {
            'url': '/demo/form/fileuploads',
            'slug': 'pages:form-fileuploads',
            'label': 'File Uploads',
          },
          {
            'url': '/demo/form/text-editors',
            'slug': 'pages:form-text-editors',
            'label': 'Text Editors',
          },
          {
            'url': '/demo/form/range-slider',
            'slug': 'pages:form-range-slider',
            'label': 'Range Slider',
          },
          {
            'url': '/demo/form/layout',
            'slug': 'pages:form-layout',
            'label': 'Layouts',
          },
          {
            'url': '/demo/form/other-plugin',
            'slug': 'pages:form-other-plugin',
            'label': 'Other Plugins',
          },
        ],
      },
      {
        'icon': 'table-column',
        'slug': 'tables',
        'label': 'Tables',
        'children': [
          {
            'url': '/demo/tables/static',
            'slug': 'pages:tables-static',
            'label': 'Static Tables',
          },
          {
            'url': '/demo/tables/custom',
            'slug': 'pages:tables-custom',
            'label': 'Custom Tables',
          },
          {
            'slug': 'datatables',
            'badge': {
              'text': '15',
              'className': 'bg-success text-white',
            },
            'label': 'DataTables',
            'children': [
              {
                'url': '/demo/tables/datatables/basic',
                'slug': 'pages:tables-datatables-basic',
                'label': 'Basic',
              },
              {
                'url': '/demo/tables/datatables/export-data',
                'slug': 'pages:tables-datatables-export-data',
                'label': 'Export Data',
              },
              {
                'url': '/demo/tables/datatables/select',
                'slug': 'pages:tables-datatables-select',
                'label': 'Select',
              },
              {
                'url': '/demo/tables/datatables/ajax',
                'slug': 'pages:tables-datatables-ajax',
                'label': 'Ajax',
              },
              {
                'url': '/demo/tables/datatables/javascript',
                'slug': 'pages:tables-datatables-javascript',
                'label': 'Javascript Source',
              },
              {
                'url': '/demo/tables/datatables/rendering',
                'slug': 'pages:tables-datatables-rendering',
                'label': 'Data Rendering',
              },
              {
                'url': '/demo/tables/datatables/scroll',
                'slug': 'pages:tables-datatables-scroll',
                'label': 'Scroll',
              },
              {
                'url': '/demo/tables/datatables/fixed-columns',
                'slug': 'pages:tables-datatables-fixed-columns',
                'label': 'Fixed Columns',
              },
              {
                'url': '/demo/tables/datatables/fixed-header',
                'slug': 'pages:tables-datatables-fixed-header',
                'label': 'Fixed Header',
              },
              {
                'url': '/demo/tables/datatables/columns',
                'slug': 'pages:tables-datatables-columns',
                'label': 'Show & Hide Column',
              },
              {
                'url': '/demo/tables/datatables/child-rows',
                'slug': 'pages:tables-datatables-child-rows',
                'label': 'Child Rows',
              },
              {
                'url': '/demo/tables/datatables/column-searching',
                'slug': 'pages:tables-datatables-column-searching',
                'label': 'Column Searching',
              },
              {
                'url': '/demo/tables/datatables/range-search',
                'slug': 'pages:tables-datatables-range-search',
                'label': 'Range Search',
              },
              {
                'url': '/demo/tables/datatables/rows-add',
                'slug': 'pages:tables-datatables-rows-add',
                'label': 'Add Rows',
              },
              {
                'url': '/demo/tables/datatables/checkbox-select',
                'slug': 'pages:tables-datatables-checkbox-select',
                'label': 'Checkbox Select',
              },
            ],
          },
        ],
      },
      {
        'icon': 'icons',
        'slug': 'icons',
        'label': 'Icons',
        'children': [
          {
            'url': '/demo/icons/tabler',
            'slug': 'pages:icons-tabler',
            'label': 'Tabler',
          },
          {
            'url': '/demo/icons/lucide',
            'slug': 'pages:icons-lucide',
            'label': 'Lucide',
          },
          {
            'url': '/demo/icons/flags',
            'slug': 'pages:icons-flags',
            'label': 'Flags',
          },
        ],
      },
      {
        'icon': 'map',
        'slug': 'maps',
        'label': 'Maps',
        'children': [
          {
            'url': '/demo/maps/google',
            'slug': 'pages:maps-google',
            'label': 'Google Maps',
          },
          {
            'url': '/demo/maps/vector',
            'slug': 'pages:maps-vector',
            'label': 'Vector Maps',
          },
          {
            'url': '/demo/maps/leaflet',
            'slug': 'pages:maps-leaflet',
            'label': 'Leaflet Maps',
          },
        ],
      },
    ],
  },
  {
    'icon': 'sitemap',
    'slug': 'menu-items',
    'label': 'Menu Items',
    'isTitle': true,
    'children': [
      {
        'icon': 'sitemap',
        'slug': 'menu-levels',
        'label': 'Menu Levels',
        'children': [
          {
            'slug': 'second-level',
            'label': 'Second Level',
            'children': [
              {
                'slug': 'menu-item-1',
                'label': 'Item 2.1',
              },
              {
                'slug': 'menu-item-2',
                'label': 'Item 2.2',
              },
            ],
          },
          {
            'slug': 'second-level-2',
            'label': 'Second Level',
            'children': [
              {
                'slug': 'menu-item-3',
                'label': 'Item 2.1',
              },
              {
                'slug': 'menu-item-4',
                'label': 'Item 2.2',
                'children': [
                  {
                    'slug': 'menu-item-5',
                    'label': 'Item 3.1',
                  },
                  {
                    'slug': 'menu-item-6',
                    'label': 'Item 3.2',
                  },
                ],
              },
            ],
          },
        ],
      },
      {
        'icon': 'ban',
        'slug': 'disabled-menu',
        'label': 'Disabled Menu',
        'isDisabled': true,
      },
      {
        'icon': 'star',
        'slug': 'special-menu',
        'label': 'Special Menu',
        'isSpecial': true,
      },
    ],
  },
]
