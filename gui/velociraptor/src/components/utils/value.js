import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import ReactJson from 'react-json-view';


export default class VeloValueRenderer extends React.Component {
    static propTypes = {
        value: PropTypes.any,
    };

    render() {
        let v = this.props.value;

        if (_.isString(v)) {
            return <>{v}</>;
        }

        if (_.isNumber(v)) {
            return JSON.stringify(v);
        }

        if (_.isNull(v)) {
            return "";
        }

        return (
            <ReactJson name={false}
                       collapsed={1}
                       collapseStringsAfterLength={100}
                       displayObjectSize={false}
                       displayDataTypes={false}
                       src={v} />
        );
    }
};
