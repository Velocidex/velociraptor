import './spinner.css';

import React from 'react';
import PropTypes from 'prop-types';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

export default class Spinner extends React.Component {
    static propTypes = {
        loading: PropTypes.bool
    };

    render() {
        if (!this.props.loading) {
            return <></>;
        }

        return (
              <div className="overlay row">
                <div className="col-2"/>
                <div className="col-8">
                  <FontAwesomeIcon
                    icon="spinner" size="lg" spin={true} />
                </div>
              </div>
        );
    }
};
