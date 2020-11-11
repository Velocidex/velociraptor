import "./time.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import moment from 'moment/moment.js';
import Tooltip from 'react-bootstrap/Tooltip';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import humanizeDuration from "humanize-duration";

const renderToolTip = (props, usec) => {
    let now = new Date().getTime();
    let difference = (now-usec);
    return <Tooltip {...props}>
             {humanizeDuration(difference, { round: true })} ago
           </Tooltip>;
};

class VeloTimestamp extends Component {
    static propTypes = {
        usec: PropTypes.number,
    }

    render() {
        let ts = this.props.usec || 0;
        var when = !ts ? moment() : moment(ts);
        return <OverlayTrigger
                 delay={{show: 250, hide: 400}}
                 overlay={(props)=>renderToolTip(props, this.props.usec)}>
                 <div className="timestamp">
                   {when.utc().format('YYYY-MM-DD HH:mm:ss') + ' UTC'}
                 </div>
               </OverlayTrigger>;
    };
}

export default VeloTimestamp;
