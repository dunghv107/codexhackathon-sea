import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter, Navigate, RouterProvider } from 'react-router'
import './index.css'
import { BuyerLayout, RootLayout, SellerLayout, PortalGuard } from './layouts'
import DemoPhone from './pages/DemoPhone'
import BuyerAddresses from './pages/buyer/Addresses'
import BuyerCheckout from './pages/buyer/Checkout'
import BuyerOrderDetail from './pages/buyer/OrderDetail'
import BuyerOrders from './pages/buyer/Orders'
import Catalog from './pages/Catalog'
import Login from './pages/Login'
import SellerDashboard from './pages/seller/Dashboard'
import SellerOrderDetail from './pages/seller/OrderDetail'
import SellerOrders from './pages/seller/Orders'
import SellerProducts from './pages/seller/Products'
import SellerShops from './pages/seller/Shops'

const router = createBrowserRouter([
  {
    element: <RootLayout />,
    children: [
      { path: '/', element: <Catalog /> },
      { path: '/shops/:shopId', element: <Catalog /> },
      { path: '/login', element: <Login /> },
      { path: '/demo/phone', element: <PortalGuard role="seller"><DemoPhone /></PortalGuard> },
      {
        path: '/buyer',
        element: <BuyerLayout />,
        children: [
          { index: true, element: <Navigate to="orders" replace /> },
          { path: 'addresses', element: <BuyerAddresses /> },
          { path: 'checkout', element: <BuyerCheckout /> },
          { path: 'orders', element: <BuyerOrders /> },
          { path: 'orders/:orderId', element: <BuyerOrderDetail /> },
        ],
      },
      {
        path: '/seller',
        element: <SellerLayout />,
        children: [
          { index: true, element: <Navigate to="dashboard" replace /> },
          { path: 'shops', element: <SellerShops /> },
          { path: 'dashboard', element: <SellerDashboard /> },
          { path: 'products', element: <SellerProducts /> },
          { path: 'orders', element: <SellerOrders /> },
          { path: 'orders/:orderId', element: <SellerOrderDetail /> },
        ],
      },
      { path: '*', element: <p>Không tìm thấy trang.</p> },
    ],
  },
])

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
)
