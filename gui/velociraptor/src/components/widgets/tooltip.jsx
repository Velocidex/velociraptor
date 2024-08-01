import PropTypes from 'prop-types';
import React from 'react';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import Tooltip from 'react-bootstrap/Tooltip';

import "./tooltip.css";

const ToolTip = ({ id, children, tooltip, placement }) => {
    if (!tooltip) {
        return <>{children}</>;
    }
    return (
        <OverlayTrigger
          placement={placement || "top"}
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

ToolTip.propTypes = {
    id: PropTypes.string,
    children: PropTypes.node.isRequired,
    tooltip: PropTypes.any,
    placement: PropTypes.string,
};

export default ToolTip;
