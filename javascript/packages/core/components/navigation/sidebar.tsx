import React from 'react';
import { Block } from 'baseui/block';
import { HeadingSmall, LabelMedium } from 'baseui/typography';
import { useNavigate, useLocation } from 'react-router-dom';

interface NavItemProps {
  icon: string;
  label: string;
  path: string;
  isActive?: boolean;
  onClick?: () => void;
}

function NavItem({ icon, label, path, isActive, onClick }: NavItemProps) {
  return (
    <Block
      display="flex"
      alignItems="center"
      padding="scale400"
      marginBottom="scale200"
      backgroundColor={isActive ? '#EBF4FF' : 'transparent'}
      $style={{
        cursor: 'pointer',
        borderRadius: '8px',
        border: isActive ? '1px solid #1B7CFC' : '1px solid transparent',
        ':hover': {
          backgroundColor: isActive ? '#EBF4FF' : '#F8FAFC',
        }
      }}
      onClick={onClick}
    >
      <Block marginRight="scale400" font="font600">
        {icon}
      </Block>
      <LabelMedium
        margin="0"
        color={isActive ? 'primary' : 'contentPrimary'}
      >
        {label}
      </LabelMedium>
    </Block>
  );
}

export function Sidebar() {
  const navigate = useNavigate();
  const location = useLocation();

  const navigationItems = [
    { icon: '📊', label: 'Dashboard', path: '/dashboard' },
    { icon: '🚀', label: 'Deployments', path: '/deployments' },
    { icon: '🤖', label: 'Models', path: '/models' },
    { icon: '📈', label: 'Monitoring', path: '/monitoring' },
    { icon: '⚙️', label: 'Workflows', path: '/workflows' },
    { icon: '👥', label: 'Projects', path: '/projects' },
  ];

  return (
    <Block
      width="240px"
      height="calc(100vh - 48px)"
      backgroundColor="#FAFBFC"
      borderRight="1px solid #E2E8F0"
      padding="scale600"
      position="fixed"
      top="48px"
      left="0"
      zIndex={10}
    >
      <Block marginBottom="scale800">
        <HeadingSmall margin="0" marginBottom="scale600">
          Navigation
        </HeadingSmall>
        
        {navigationItems.map((item) => (
          <NavItem
            key={item.path}
            icon={item.icon}
            label={item.label}
            path={item.path}
            isActive={location.pathname === item.path}
            onClick={() => navigate(item.path)}
          />
        ))}
      </Block>

      <Block marginTop="scale800">
        <HeadingSmall margin="0" marginBottom="scale600" color="contentSecondary">
          Quick Actions
        </HeadingSmall>
        
        <NavItem
          icon="➕"
          label="Deploy Model"
          path="/deploy"
          onClick={() => navigate('/deploy')}
        />
        
        <NavItem
          icon="📤"
          label="Upload Model"
          path="/upload"
          onClick={() => navigate('/upload')}
        />
      </Block>
    </Block>
  );
}