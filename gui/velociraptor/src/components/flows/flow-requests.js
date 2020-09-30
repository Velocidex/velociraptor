import React from 'react';
import PropTypes from 'prop-types';

import VeloAce from '../core/ace.js';

export default class FlowRequests extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    render() {
        let serialized = JSON.stringify(this.props.flow.request, null, 2);
        let options = {
            readOnly: true,
        };

        return (
            <div className="card panel" >
              <h5 className="card-header">Request sent to client</h5>
              <div className="card-body">
                <VeloAce text={serialized}
                         options={options} />
              </div>
            </div>

        );
    }
};
