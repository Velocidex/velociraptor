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
        var ts;
        // Maybe the timestamp is specified as an iso
        if (_.isString(this.props.iso)) {
            let parsed = new Date(this.props.iso);
            if (parsed.getTime() === parsed.getTime()) {
                ts = parsed;
            }

        }

        // If the timestamp is a number then it might be in usec
        if (!ts && _.isNumber(this.props.usec) && this.props.usec>0) {
            let parsed = new Date(this.props.usec);
            if (parsed.getTime() === parsed.getTime()) {
                ts = parsed;
            } else {
                // Or maybe in seconds since epoch.
                let parsed = new Date(this.props.usec * 1000);
                if (parsed.getTime() === parsed.getTime()) {
                    ts = parsed;
                }
            }
        }

        // Could not parse it - just return what we got.
        if (!ts) {
            return JSON.stringify(this.props.iso);
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
