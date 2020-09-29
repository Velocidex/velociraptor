import { Component } from 'react';
import PropTypes from 'prop-types';
import moment from 'moment/moment.js';

class VeloTimestamp extends Component {
    static propTypes = {
        usec: PropTypes.number,
    }

    render() {
        let ts = this.props.usec || 0;
        var when = !ts ? moment() : moment(ts);
        return when.utc().format('YYYY-MM-DD HH:mm:ss') + ' UTC';
    };
}

export default VeloTimestamp;
