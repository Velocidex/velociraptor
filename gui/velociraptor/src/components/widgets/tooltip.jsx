import PropTypes from 'prop-types';
import React from 'react';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import Tooltip from 'react-bootstrap/Tooltip';

import "./tooltip.css";


export default class ToolTip extends React.Component {
    static propTypes = {
        id: PropTypes.string,
        children: PropTypes.node.isRequired,
        tooltip: PropTypes.any,
        placement: PropTypes.string,
    }

    render() {
        if (!this.props.tooltip) {
            return <>{this.props.children}</>;
        }

        let tp = <Tooltip id={this.props.tooltip}>
                   {this.props.tooltip}
                 </Tooltip>;

        return (
            <OverlayTrigger
              placement={this.props.placement || "top"}
              overlay={tp}>
              {this.props.children}
            </OverlayTrigger>
        );
    }
}
