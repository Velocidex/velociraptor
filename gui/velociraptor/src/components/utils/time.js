import "./time.css";
import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import moment from 'moment/moment.js';
import Tooltip from 'react-bootstrap/Tooltip';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import humanizeDuration from "humanize-duration";

const renderToolTip = (props, ts) => {
    let now = new Date().getTime();
    let difference = (now-ts.getTime());
    return <Tooltip {...props}>
             {humanizeDuration(difference, { round: true })} ago
           </Tooltip>;
};

class VeloTimestamp extends Component {
    static propTypes = {
        usec: PropTypes.number,
        iso: PropTypes.any,
    }

    render() {
        let ts = new Date(this.props.usec || 0);

        if (_.isString(this.props.iso)) {
            ts = new Date(this.props.iso);
        }

        if (!ts) {
            return <></>;
        }

        var when = moment(ts);
        return <OverlayTrigger
                 delay={{show: 250, hide: 400}}
                 overlay={(props)=>renderToolTip(props, ts)}>
                 <div className="timestamp">
                   {when.utc().format('YYYY-MM-DD HH:mm:ss') + ' UTC'}
                 </div>
               </OverlayTrigger>;
    };
}

export default VeloTimestamp;
