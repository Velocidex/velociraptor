import "./number.css";

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

export default class NumberFormatter extends React.Component {
    static propTypes = {
        value: PropTypes.any,
    };

    render() {
        return (
            <div className="numeric">
            { _.isNumber(this.props.value) &&
              JSON.stringify(this.props.value)
            }
            </div>
        );
    }
};
