import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import Tooltip from 'react-bootstrap/Tooltip';

import "./tooltip.css";

const ToolTip = ({ id, children, tooltip }) => {
    return (
        <OverlayTrigger
          overlay={
              <Tooltip id={id}>
                {tooltip}
                <span className="sr-only">
                  {tooltip}
                </span>
              </Tooltip>
          }>
          {children}
        </OverlayTrigger>
    );
};

export default ToolTip;
