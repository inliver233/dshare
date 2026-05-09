import type { CSSProperties, ReactNode } from 'react';
import { motion } from 'framer-motion';
import type { HTMLMotionProps } from 'framer-motion';

type GlassPanelProps = Omit<HTMLMotionProps<'div'>, 'children'> & {
  children: ReactNode;
  sx?: CSSProperties;
};

export function GlassPanel({ children, sx, style, ...props }: GlassPanelProps) {
  return (
    <motion.div
      whileHover={{
        boxShadow: '0 8px 32px rgba(0,119,182,0.3)',
        borderColor: 'rgba(72,202,228,0.4)',
      }}
      style={{
        background: 'rgba(15, 23, 42, 0.45)',
        backdropFilter: 'blur(24px) saturate(180%)',
        WebkitBackdropFilter: 'blur(24px) saturate(180%)',
        border: '1px solid rgba(255,255,255,0.15)',
        borderRadius: 16,
        boxShadow: '0 4px 24px rgba(0,0,0,0.4)',
        padding: '2rem',
        ...sx,
        ...style,
      }}
      {...props}
    >
      {children}
    </motion.div>
  );
}
