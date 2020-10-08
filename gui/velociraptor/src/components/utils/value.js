import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

export default class VeloValueRenderer extends React.Component {
    static propTypes = {
        value: PropTypes.any,
    };

    render() {
        let v = this.props.value;

        if (_.isString(v)) {
            return <>{v}</>;
        }
        return (
            <>
              {JSON.stringify(v)}
            </>
        );
    }
};
