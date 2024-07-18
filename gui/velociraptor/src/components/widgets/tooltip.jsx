import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import Tooltip from 'react-bootstrap/Tooltip';

import "./tooltip.css";

const ToolTip = ({ id, children, tooltip }) => (
    <OverlayTrigger
      overlay={<Tooltip id={id}>{tooltip}</Tooltip>}>
      {children}
    </OverlayTrigger>
);

export default ToolTip;
