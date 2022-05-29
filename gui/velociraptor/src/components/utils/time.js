import "./time.css";
import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import moment from 'moment/moment.js';
import Tooltip from 'react-bootstrap/Tooltip';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import T from '../i8n/i8n.js';

const renderToolTip = (props, ts) => {
    let now = new Date().getTime();
    let difference = (now-ts.getTime());
    return <Tooltip {...props}>
             {T("HumanizeDuration", difference)}
           </Tooltip>;
};

const digitsRegex = /^[0-9.]+$/;

// Try hard to convert the value into a proper timestamp.
export const ToStandardTime = value => {
    // Maybe the timestamp is specified as an iso string
    if (_.isString(value)) {
        // Really an int that got converted to a string along the way
        // (can happen with 64 bit ints which JSON does not support).
        if (digitsRegex.test(value)) {
            value = parseFloat(value);

        } else {
            // Maybe an iso string
            let parsed = new Date(value);
            if (!_.isNaN(parsed.getTime())) {

                // Ok this is fine.
                return parsed;
            };
        }
    }

    // If the timestamp is anumber then it might be in sec, msec,
    // usec or nsec - we want to support all of those.
    if (!_.isNaN(value) &&  _.isNumber(value) && value > 0) {
        // Maybe nsec
        if (value > 20000000000000000) {  // 11 October 2603 in microsec
            value /= 1000000;  // To msec

        } else if (value > 20000000000000) {  // 11 October 2603 in milliseconds
            value /= 1000;

        } else if (value > 20000000000) { // 11 October 2603 in seconds
            // Already in msec.

        } else {
            // Might be in sec - convert to ms.
            value = value * 1000;
        }


        let parsed = new Date(value);
        if (!_.isNaN(parsed.getTime())) {
            return parsed;
        };
    }
    return value;
};

class VeloTimestamp extends Component {
    static propTypes = {
        usec: PropTypes.any,
        iso: PropTypes.any,
    }

    render() {
        let value = this.props.iso || this.props.usec;
        let ts = ToStandardTime(value);

        if (_.isNaN(ts)) {
            return <></>;
        }

        // Could not parse it - just return what we got.
        if (!ts) {
            return <div>{value && JSON.stringify(value)}</div>;
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
